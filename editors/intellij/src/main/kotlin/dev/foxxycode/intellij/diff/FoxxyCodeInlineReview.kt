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
import com.intellij.openapi.editor.colors.EditorFontType
import com.intellij.openapi.editor.event.DocumentEvent
import com.intellij.openapi.editor.event.DocumentListener
import com.intellij.openapi.editor.event.EditorFactoryEvent
import com.intellij.openapi.editor.event.EditorFactoryListener
import com.intellij.openapi.editor.event.EditorMouseEvent
import com.intellij.openapi.editor.event.EditorMouseListener
import com.intellij.openapi.editor.markup.HighlighterLayer
import com.intellij.openapi.editor.markup.HighlighterTargetArea
import com.intellij.openapi.editor.markup.RangeHighlighter
import com.intellij.openapi.editor.markup.TextAttributes
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.ui.JBColor
import com.intellij.util.ui.JBUI
import dev.foxxycode.intellij.FoxxyCodeBundle
import java.awt.Color
import java.awt.Graphics
import java.awt.Rectangle
import java.awt.event.MouseEvent
import java.util.IdentityHashMap

/** Project-local review state for writable source editors. Lives on the EDT. */
internal class FoxxyCodeInlineReview(private val project: Project, parent: Disposable) {
    private data class Block(val id: Long, val hunk: ReviewHunk)
    private data class Presentation(val inlays: List<Inlay<ReviewActions>>, val marks: List<RangeHighlighter>)

    private val factory = EditorFactory.getInstance()
    private val blocks = IdentityHashMap<Document, List<Block>>()
    private val presentations = IdentityHashMap<Editor, Presentation>()
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
                val hit = presentations[event.editor]?.inlays?.firstOrNull {
                    it.bounds?.contains(event.mouseEvent.point) == true
                } ?: return
                val bounds = hit.bounds ?: return
                val reject = hit.renderer.rejectAt(event.mouseEvent.x - bounds.x, event.editor) ?: return
                event.consume()
                resolve(event.editor.document, hit.renderer.id, reject)
            }
        }, parent)
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
        private val accept = FoxxyCodeBundle.message("diff.action.accept")
        private val reject = FoxxyCodeBundle.message("diff.action.reject")
        private val padding get() = JBUI.scale(6)
        private val gap get() = JBUI.scale(8)
        private val good = JBColor(Color(0x1B, 0x6B, 0x3C), Color(0x88, 0xD5, 0xA5))
        private val bad = JBColor(Color(0x9C, 0x32, 0x2B), Color(0xF0, 0xA0, 0x98))

        private fun font(editor: Editor) = editor.colorsScheme.getFont(EditorFontType.PLAIN)
        private fun acceptWidth(editor: Editor) = editor.contentComponent.getFontMetrics(font(editor)).stringWidth(accept) + 2 * padding

        override fun calcWidthInPixels(inlay: Inlay<*>): Int {
            val editor = inlay.editor
            return acceptWidth(editor) + gap + editor.contentComponent.getFontMetrics(font(editor)).stringWidth(reject) + 2 * padding
        }

        fun rejectAt(x: Int, editor: Editor): Boolean? = when {
            x < acceptWidth(editor) -> false
            x >= acceptWidth(editor) + gap -> true
            else -> null
        }

        override fun paint(inlay: Inlay<*>, g: Graphics, targetRegion: Rectangle, textAttributes: TextAttributes) {
            val graphics = g.create()
            try {
                graphics.font = font(inlay.editor)
                val metrics = graphics.fontMetrics
                val baseline = targetRegion.y + (targetRegion.height - metrics.height) / 2 + metrics.ascent
                graphics.color = good
                graphics.drawString(accept, targetRegion.x + padding, baseline)
                graphics.color = bad
                graphics.drawString(reject, targetRegion.x + acceptWidth(inlay.editor) + gap + padding, baseline)
            } finally {
                graphics.dispose()
            }
        }
    }
}
