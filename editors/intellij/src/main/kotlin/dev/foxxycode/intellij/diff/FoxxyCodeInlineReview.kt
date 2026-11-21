package dev.foxxycode.intellij.diff

import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.diff.DiffColors
import com.intellij.openapi.editor.Document
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.EditorCustomElementRenderer
import com.intellij.openapi.editor.EditorFactory
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.event.DocumentEvent
import com.intellij.openapi.editor.event.DocumentListener
import com.intellij.openapi.editor.event.EditorFactoryEvent
import com.intellij.openapi.editor.event.EditorFactoryListener
import com.intellij.openapi.editor.event.EditorMouseEvent
import com.intellij.openapi.editor.event.EditorMouseEventArea
import com.intellij.openapi.editor.event.EditorMouseListener
import com.intellij.openapi.editor.event.EditorMouseMotionListener
import com.intellij.openapi.editor.ex.EditorEx
import com.intellij.openapi.editor.markup.HighlighterLayer
import com.intellij.openapi.editor.markup.HighlighterTargetArea
import com.intellij.openapi.editor.markup.RangeHighlighter
import com.intellij.openapi.editor.markup.TextAttributes
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.ui.ColorUtil
import com.intellij.util.ui.JBUI
import dev.foxxycode.intellij.FoxxyCodeBundle
import java.awt.BasicStroke
import java.awt.Color
import java.awt.Cursor
import java.awt.Graphics
import java.awt.Graphics2D
import java.awt.Point
import java.awt.Rectangle
import java.awt.RenderingHints
import java.awt.event.MouseEvent
import java.awt.geom.Path2D
import java.awt.geom.RoundRectangle2D
import java.util.IdentityHashMap

/** Project-local review state for writable source editors. Lives on the EDT. */
internal class FoxxyCodeInlineReview(private val project: Project, parent: Disposable) {
    private data class Block(val id: Long, val hunk: ReviewHunk)
    private data class Presentation(val inlays: List<Inlay<ReviewActions>>, val marks: List<RangeHighlighter>)
    private data class ReviewButton(val inlay: Inlay<ReviewActions>, val reject: Boolean)

    private val factory = EditorFactory.getInstance()
    private val blocks = IdentityHashMap<Document, List<Block>>()
    private val presentations = IdentityHashMap<Editor, Presentation>()
    private var hoveredButton: ReviewButton? = null
    private var nextId = 0L
    private val log = logger<FoxxyCodeInlineReview>()

    init {
        factory.eventMulticaster.addDocumentListener(object : DocumentListener {
            override fun documentChanged(event: DocumentEvent) {
                val old = blocks[event.document] ?: return
                val updated = old.mapNotNull { block ->
                    val moved = block.hunk.move(event.offset, event.offset + event.oldLength, event.newLength)
                    moved?.takeIf { it.matches(event.document.immutableCharSequence) }?.let { block.copy(hunk = it) }
                }
                if (updated.isEmpty()) blocks.remove(event.document) else blocks[event.document] = updated
                // The editor updates its inlay positions as part of the document event.
                ApplicationManager.getApplication().invokeLater {
                    if (!project.isDisposed) draw(event.document)
                }
            }
        }, parent)
        factory.addEditorFactoryListener(object : EditorFactoryListener {
            override fun editorCreated(event: EditorFactoryEvent) {
                if (event.editor.project == project && blocks.containsKey(event.editor.document)) draw(event.editor)
            }

            override fun editorReleased(event: EditorFactoryEvent) {
                val editor = event.editor
                if (editor.project != project && !presentations.containsKey(editor)) return
                clear(editor)
                if (factory.getEditors(editor.document, project).none { it !== editor && !it.isViewer && !it.isDisposed }) {
                    blocks.remove(editor.document)
                }
            }
        }, parent)
        factory.eventMulticaster.addEditorMouseListener(object : EditorMouseListener {
            override fun mousePressed(event: EditorMouseEvent) {
                if (event.mouseEvent.button != MouseEvent.BUTTON1) return
                val hit = buttonAt(event) ?: return
                event.consume()
                resolve(event.editor.document, hit.inlay.renderer.id, hit.reject)
            }

            override fun mouseExited(event: EditorMouseEvent) {
                if (hoveredButton?.inlay?.editor === event.editor) setHover(null)
            }
        }, parent)
        factory.eventMulticaster.addEditorMouseMotionListener(object : EditorMouseMotionListener {
            override fun mouseMoved(event: EditorMouseEvent) = setHover(buttonAt(event))

            override fun mouseDragged(event: EditorMouseEvent) = setHover(null)
        }, parent)
        factory.eventMulticaster.addVisibleAreaListener({ event ->
            if (hoveredButton?.inlay?.editor === event.editor) setHover(null)
        }, parent)
    }

    private fun buttonAt(event: EditorMouseEvent): ReviewButton? {
        if (event.area != EditorMouseEventArea.EDITING_AREA) return null
        val inlays = presentations[event.editor]?.inlays ?: return null
        val point = event.mouseEvent.point
        for (inlay in inlays) {
            val bounds = inlay.bounds ?: continue
            if (!bounds.contains(point)) continue
            val reject = inlay.renderer.rejectAt(point, bounds, event.editor) ?: continue
            return ReviewButton(inlay, reject)
        }
        return null
    }

    private fun setHover(button: ReviewButton?) {
        val previous = hoveredButton
        if (previous == button) return
        hoveredButton = button

        previous?.inlay?.let { inlay ->
            inlay.renderer.hoveredReject = null
            if (inlay !== button?.inlay && inlay.isValid) inlay.repaint()
        }
        button?.inlay?.let { inlay ->
            inlay.renderer.hoveredReject = button.reject
            if (inlay.isValid) inlay.repaint()
        }

        val previousEditor = previous?.inlay?.editor as? EditorEx
        val editor = button?.inlay?.editor as? EditorEx
        if (previousEditor !== editor) {
            if (previousEditor != null && !previousEditor.isDisposed) previousEditor.setCustomCursor(this, null)
            editor?.setCustomCursor(this, Cursor.getPredefinedCursor(Cursor.HAND_CURSOR))
        }
    }

    fun offer(editor: Editor, ev: FoxxyCodeEditEvent, replace: Boolean): Boolean {
        val doc = editor.document
        val before = editorReviewText(ev.before)
        val after = editorReviewText(ev.after)
        if (!doc.isWritable || editor.isViewer || '\uFFFD' in before || '\uFFFD' in after ||
            !after.contentEquals(doc.immutableCharSequence)
        ) return false
        val additions = reviewHunks(before, after).map { Block(++nextId, it) }
        if (additions.isEmpty()) return false
        val existing = if (replace) emptyList() else blocks[doc].orEmpty()
        val retained = existing.filter { block ->
            additions.none { it.hunk.overlaps(block.hunk) }
        }
        blocks[doc] = retained + additions
        draw(doc)
        if (replace) {
            editor.caretModel.moveToOffset(additions.first().hunk.start)
            editor.scrollingModel.scrollToCaret(com.intellij.openapi.editor.ScrollType.CENTER)
        }
        return true
    }

    private fun resolve(doc: Document, id: Long, reject: Boolean) {
        val block = blocks[doc]?.firstOrNull { it.id == id } ?: return
        val hunk = block.hunk
        if (!hunk.matches(doc.immutableCharSequence)) return
        if (reject) {
            if (!doc.isWritable) return
            try {
                WriteCommandAction.runWriteCommandAction(
                    project, FoxxyCodeBundle.message("diff.command.rejectBlock"), null, {
                        doc.replaceString(hunk.start, hunk.end, hunk.before)
                    },
                )
            } catch (ex: RuntimeException) {
                log.warn("Could not reject inline edit block", ex)
                return
            }
        }
        val remaining = blocks[doc].orEmpty().filter { it.id != id }
        if (remaining.isEmpty()) blocks.remove(doc) else blocks[doc] = remaining
        draw(doc)
    }

    private fun draw(doc: Document) {
        for (editor in factory.getEditors(doc, project)) draw(editor)
    }

    private fun draw(editor: Editor) {
        clear(editor)
        if (editor.isDisposed || editor.isViewer) return
        val doc = editor.document
        val hunks = blocks[doc] ?: return
        val inlays = ArrayList<Inlay<ReviewActions>>()
        val marks = ArrayList<RangeHighlighter>()
        for (block in hunks) {
            val hunk = block.hunk
            val actions = ReviewActions(block.id)
            // Put the actions above the first affected line, or above the deletion anchor.
            editor.inlayModel.addBlockElement(hunk.start, false, true, 0, actions)?.let(inlays::add)
            if (hunk.after.isNotEmpty()) {
                marks.add(editor.markupModel.addRangeHighlighter(
                    if (hunk.before.isEmpty()) DiffColors.DIFF_INSERTED else DiffColors.DIFF_MODIFIED,
                    hunk.start, hunk.end - 1, HighlighterLayer.ADDITIONAL_SYNTAX,
                    HighlighterTargetArea.LINES_IN_RANGE,
                ))
            }
        }
        presentations[editor] = Presentation(inlays, marks)
    }

    private fun clear(editor: Editor) {
        val shown = presentations.remove(editor) ?: return
        if (hoveredButton?.inlay?.editor === editor) setHover(null)
        shown.inlays.filter { it.isValid }.forEach { Disposer.dispose(it) }
        if (!editor.isDisposed) {
            shown.marks.filter { it.isValid }.forEach(editor.markupModel::removeHighlighter)
        }
    }

    fun clear() {
        presentations.keys.toList().forEach(::clear)
        blocks.clear()
    }

    private class ReviewActions(val id: Long) : EditorCustomElementRenderer {
        private data class Buttons(val accept: Rectangle, val reject: Rectangle)

        var hoveredReject: Boolean? = null

        private val accept = FoxxyCodeBundle.message("diff.action.accept")
        private val reject = FoxxyCodeBundle.message("diff.action.reject")
        private val padding get() = JBUI.scale(8)
        private val inset get() = JBUI.scale(2)
        private val gap get() = JBUI.scale(5)
        private val iconSize get() = JBUI.scale(12)
        private val font get() = JBUI.Fonts.smallFont()

        private fun buttons(editor: Editor, x: Int = 0, y: Int = 0): Buttons {
            val metrics = editor.contentComponent.getFontMetrics(font)
            val height = metrics.height + JBUI.scale(4)
            val decorationWidth = 2 * padding + iconSize + gap
            val first = Rectangle(x + inset, y + inset, metrics.stringWidth(accept) + decorationWidth, height)
            val second = Rectangle(first.x + first.width + gap, first.y, metrics.stringWidth(reject) + decorationWidth, height)
            return Buttons(first, second)
        }

        override fun calcWidthInPixels(inlay: Inlay<*>): Int {
            val last = buttons(inlay.editor).reject
            return last.x + last.width + inset
        }

        override fun calcHeightInPixels(inlay: Inlay<*>): Int = buttons(inlay.editor).accept.height + 2 * inset

        fun rejectAt(point: Point, area: Rectangle, editor: Editor): Boolean? {
            val (acceptBounds, rejectBounds) = buttons(editor, area.x, area.y)
            return when {
                acceptBounds.contains(point) -> false
                rejectBounds.contains(point) -> true
                else -> null
            }
        }

        override fun paint(inlay: Inlay<*>, g: Graphics, targetRegion: Rectangle, textAttributes: TextAttributes) {
            val graphics = g.create() as Graphics2D
            try {
                val scheme = inlay.editor.colorsScheme
                val dark = ColorUtil.isDark(scheme.defaultBackground)
                val accent = if (dark) Color(0xC084FC) else Color(0x9333EA)
                val danger = if (dark) Color(0xF87171) else Color(0xB91C1C)
                val (acceptBounds, rejectBounds) = buttons(inlay.editor, targetRegion.x, targetRegion.y)

                graphics.setRenderingHint(RenderingHints.KEY_ANTIALIASING, RenderingHints.VALUE_ANTIALIAS_ON)
                graphics.font = font
                paintButton(graphics, acceptBounds, accept, scheme.defaultForeground, accent, reject = false)
                paintButton(graphics, rejectBounds, reject, danger, danger, reject = true)
            } finally {
                graphics.dispose()
            }
        }

        private fun paintButton(
            g: Graphics2D,
            bounds: Rectangle,
            label: String,
            foreground: Color,
            tint: Color,
            reject: Boolean,
        ) {
            val stroke = JBUI.scale(1.5f)
            val corner = bounds.height - stroke
            val shape = RoundRectangle2D.Float(
                bounds.x + stroke / 2,
                bounds.y + stroke / 2,
                bounds.width - stroke,
                bounds.height - stroke,
                corner,
                corner,
            )
            val hovered = hoveredReject == reject
            val fillAlpha = when {
                hovered -> 70
                reject -> 18
                else -> 31
            }
            g.color = Color(tint.red, tint.green, tint.blue, fillAlpha)
            g.fill(shape)
            g.color = Color(tint.red, tint.green, tint.blue, if (hovered) 230 else 165)
            g.stroke = BasicStroke(stroke)
            g.draw(shape)

            val iconX = bounds.x + padding
            val iconY = bounds.y + (bounds.height - iconSize) / 2
            val metrics = g.fontMetrics
            val baseline = bounds.y + (bounds.height - metrics.height) / 2 + metrics.ascent
            g.color = foreground
            g.drawString(label, iconX + iconSize + gap, baseline)
            paintIcon(g, iconX, iconY, reject)
        }

        private fun paintIcon(g: Graphics2D, x: Int, y: Int, reject: Boolean) {
            val icon = g.create() as Graphics2D
            try {
                icon.translate(x, y)
                val scale = iconSize / 12.0
                icon.scale(scale, scale)
                icon.stroke = BasicStroke(1.35f, BasicStroke.CAP_ROUND, BasicStroke.JOIN_ROUND)
                val path = Path2D.Float()
                if (reject) {
                    path.moveTo(4.5, 6.5)
                    path.lineTo(2.0, 4.0)
                    path.lineTo(4.5, 1.5)
                    path.moveTo(2.0, 4.0)
                    path.lineTo(7.25, 4.0)
                    path.curveTo(10.85, 4.0, 10.85, 9.5, 7.25, 9.5)
                    path.lineTo(5.0, 9.5)
                } else {
                    path.moveTo(2.0, 6.0)
                    path.lineTo(5.0, 9.0)
                    path.lineTo(10.0, 3.0)
                }
                icon.draw(path)
            } finally {
                icon.dispose()
            }
        }
    }
}
