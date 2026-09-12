package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.EditorCustomElementRenderer
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.markup.TextAttributes
import java.awt.Graphics
import java.awt.Rectangle

/** Draws the lines of a suggestion that fall below the caret's own line. */
class BlockSuggestionRenderer(
    private val editor: Editor,
    private val lines: List<String>,
) : EditorCustomElementRenderer {

    override fun calcWidthInPixels(inlay: Inlay<*>): Int {
        val metrics = editor.contentComponent.getFontMetrics(SuggestionPainter.font(editor))
        return lines.maxOfOrNull { metrics.stringWidth(it) } ?: 0
    }

    override fun calcHeightInPixels(inlay: Inlay<*>): Int = editor.lineHeight * lines.size

    override fun paint(inlay: Inlay<*>, g: Graphics, targetRegion: Rectangle, textAttributes: TextAttributes) {
        val font = SuggestionPainter.font(editor)
        g.color = SuggestionPainter.color()
        g.font = font
        val ascent = editor.contentComponent.getFontMetrics(font).ascent
        lines.forEachIndexed { i, line ->
            g.drawString(line, 0, targetRegion.y + i * editor.lineHeight + ascent)
        }
    }
}
