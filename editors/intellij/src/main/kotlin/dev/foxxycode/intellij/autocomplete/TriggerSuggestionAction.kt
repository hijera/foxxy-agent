package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.editor.Editor

/**
 * Asks for a suggestion on demand (Alt+\ by default). This is the whole interaction when
 * autocomplete.trigger is "manual", and a way to force one without waiting for the debounce when it
 * is "auto".
 */
class TriggerSuggestionAction : AnAction() {

    override fun actionPerformed(e: AnActionEvent) {
        val editor: Editor = e.getData(CommonDataKeys.EDITOR) ?: return
        val project = editor.project ?: return
        FoxxyCodeAutocompleteService.getInstance(project).triggerManually(editor)
    }

    override fun update(e: AnActionEvent) {
        val editor = e.getData(CommonDataKeys.EDITOR)
        val project = editor?.project
        e.presentation.isEnabled = project != null &&
            FoxxyCodeAutocompleteService.getInstance(project).config.enabled
    }
}
