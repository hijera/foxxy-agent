package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.EditorCustomElementRenderer
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.markup.TextAttributes
import java.awt.Graphics
import java.awt.Rectangle

/** Draws the part of a suggestion that continues the line the caret sits on. */
class InlineSuggestionRenderer(
    private val editor: Editor,
    private val text: String,
) : EditorCustomElementRenderer {

    override fun calcWidthInPixels(inlay: Inlay<*>): Int =
        editor.contentComponent.getFontMetrics(SuggestionPainter.font(editor)).stringWidth(text)

    override fun paint(inlay: Inlay<*>, g: Graphics, targetRegion: Rectangle, textAttributes: TextAttributes) {
        val font = SuggestionPainter.font(editor)
        g.color = SuggestionPainter.color()
        g.font = font
        val ascent = editor.contentComponent.getFontMetrics(font).ascent
        g.drawString(text, targetRegion.x, targetRegion.y + ascent)
    }
}
