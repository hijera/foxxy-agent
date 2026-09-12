package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.colors.EditorFontType
import com.intellij.ui.JBColor
import java.awt.Color
import java.awt.Font

/**
 * Shared look of the greyed suggestion. The platform's own inline-completion API only exists from
 * 2023.3 and this plugin still targets 222, so the suggestion is painted by hand into two inlays -
 * the same split tabnine uses. Both renderers take their font and color from here so the caret line
 * and the lines below it cannot drift apart.
 */
internal object SuggestionPainter {
    /** Italic, from the editor's own scheme, so the suggestion lines up with real code. */
    fun font(editor: Editor): Font = editor.colorsScheme.getFont(EditorFontType.ITALIC)

    /** Grey that stays readable in both light and dark editor schemes. */
    fun color(): Color = JBColor.GRAY
}
