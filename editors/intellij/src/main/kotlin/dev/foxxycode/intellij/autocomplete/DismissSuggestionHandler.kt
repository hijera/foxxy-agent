package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.actionSystem.DataContext
import com.intellij.openapi.editor.Caret
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.actionSystem.EditorActionHandler

/** Dismisses the inline suggestion with Escape, leaving Escape's other jobs intact otherwise. */
class DismissSuggestionHandler(private val original: EditorActionHandler) : EditorActionHandler() {

    override fun doExecute(editor: Editor, caret: Caret?, dataContext: DataContext) {
        val hadPreview = SuggestionPreview.get(editor) != null
        SuggestionPreview.clear(editor)
        // Escape usually has several jobs (close a popup, drop extra carets). Only swallow it when
        // there was actually a suggestion to dismiss.
        if (!hadPreview && original.isEnabled(editor, caret, dataContext)) {
            original.execute(editor, caret, dataContext)
        }
    }

    override fun isEnabledForCaret(editor: Editor, caret: Caret, dataContext: DataContext?): Boolean {
        if (SuggestionPreview.get(editor) != null) return true
        return original.isEnabled(editor, caret, dataContext)
    }
}
