package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.editor.Document
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.EditorFactory
import com.intellij.openapi.editor.EditorKind
import com.intellij.openapi.editor.event.BulkAwareDocumentListener
import com.intellij.openapi.editor.event.DocumentEvent

/**
 * Watches ordinary typing and asks [FoxxyCodeAutocompleteService] for a suggestion.
 *
 * Registered as an editorFactoryDocumentListener, so it sees every document in the IDE; everything
 * that is not a person typing into a writable main editor is filtered out here rather than costing
 * a request. Any change at all drops the suggestion currently on screen first: grey text that
 * survives an edit describes a document that no longer exists.
 */
class AutocompleteDocumentListener : BulkAwareDocumentListener {

    override fun documentChangedNonBulk(event: DocumentEvent) {
        // Document changes arrive on the EDT inside a write action. Anything else (a background
        // refactoring worker, say) is not a user typing, and touching the editor there is unsafe.
        if (!ApplicationManager.getApplication().isDispatchThread) return

        val editor = focusedEditor(event.document) ?: return
        if (editor.editorKind != EditorKind.MAIN_EDITOR) return
        val project = editor.project ?: return
        if (project.isDisposed) return

        if (!SuggestionText.isTypingChange(event.newLength, event.oldLength) || !editor.document.isWritable) {
            SuggestionPreview.clear(editor)
            return
        }

        val document = event.document
        val offset = event.offset + event.newLength
        val text = document.charsSequence
        val lineStart = document.getLineStartOffset(document.getLineNumber(offset))
        FoxxyCodeAutocompleteService.getInstance(project).onTyped(
            editor,
            offset,
            event.newFragment.toString(),
            lineBefore = text.subSequence(lineStart, offset).toString(),
            charAfter = if (offset < text.length) text[offset] else null,
        )
    }

    /**
     * The editor the user is actually typing in. One document can back several editors (split
     * panes, previews); the suggestion belongs only to the focused one.
     */
    private fun focusedEditor(document: Document): Editor? {
        if (ApplicationManager.getApplication().isDisposed) return null
        val editors = EditorFactory.getInstance().getEditors(document)
        return editors.firstOrNull { it.contentComponent.isFocusOwner }
    }
}
