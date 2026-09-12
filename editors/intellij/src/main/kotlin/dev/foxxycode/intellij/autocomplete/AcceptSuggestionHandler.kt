package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.actionSystem.DataContext
import com.intellij.openapi.editor.Caret
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.actionSystem.EditorActionHandler

/**
 * Accepts the inline suggestion with Tab.
 *
 * Implemented as an editorActionHandler wrapped around the platform's own Tab handler rather than as
 * an action bound to Tab: the wrapper answers only while a suggestion is on screen and delegates
 * otherwise, so indentation, live-template navigation and every other Tab behaviour keep working
 * untouched. (tabnine binds an action to Tab and needs an ActionPromoter to win that conflict; the
 * wrapper avoids the conflict entirely.)
 */
class AcceptSuggestionHandler(private val original: EditorActionHandler) : EditorActionHandler() {

    override fun doExecute(editor: Editor, caret: Caret?, dataContext: DataContext) {
        val preview = SuggestionPreview.get(editor)
        if (preview != null) {
            preview.apply()
            return
        }
        if (original.isEnabled(editor, caret, dataContext)) {
            original.execute(editor, caret, dataContext)
        }
    }

    override fun isEnabledForCaret(editor: Editor, caret: Caret, dataContext: DataContext?): Boolean {
        if (SuggestionPreview.get(editor) != null) return true
        return original.isEnabled(editor, caret, dataContext)
    }
}
