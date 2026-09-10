package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.Disposable
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.event.CaretEvent
import com.intellij.openapi.editor.event.CaretListener
import com.intellij.openapi.editor.ex.util.EditorUtil
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.util.Key

/**
 * The suggestion currently drawn in one editor, together with the inlays drawing it.
 *
 * At most one preview exists per editor; it lives in the editor's user data and is disposed as soon
 * as it stops being valid - the caret moves away, the document changes, or the user accepts or
 * dismisses it. Disposing removes the inlays, so nothing can leave grey text behind in a document
 * that has moved on.
 */
class SuggestionPreview private constructor(
    val editor: Editor,
    val text: String,
    val offset: Int,
) : Disposable {

    private val inlays = mutableListOf<Inlay<*>>()

    private fun render() {
        val (head, tail) = SuggestionText.split(text)
        if (head.isNotEmpty()) {
            editor.inlayModel.addInlineElement(offset, true, InlineSuggestionRenderer(editor, head))
                ?.let { inlays.add(it) }
        }
        if (tail.isNotEmpty()) {
            editor.inlayModel.addBlockElement(offset, true, false, 0, BlockSuggestionRenderer(editor, tail))
                ?.let { inlays.add(it) }
        }
    }

    /**
     * Inserts the suggestion at the caret. The preview is disposed first so its inlays cannot
     * survive the document change that follows.
     */
    fun apply() {
        val project = editor.project ?: return
        val insertAt = editor.caretModel.offset
        val toInsert = text
        clear(editor, report = false)
        FoxxyCodeAutocompleteService.getInstance(project).report("accepted")
        WriteCommandAction.runWriteCommandAction(project) {
            editor.document.insertString(insertAt, toInsert)
            editor.caretModel.moveToOffset(insertAt + toInsert.length)
        }
    }

    override fun dispose() {
        inlays.forEach { Disposer.dispose(it) }
        inlays.clear()
        if (editor.getUserData(KEY) === this) {
            editor.putUserData(KEY, null)
        }
    }

    companion object {
        private val KEY = Key.create<SuggestionPreview>("foxxycode.autocomplete.preview")

        fun get(editor: Editor): SuggestionPreview? = editor.getUserData(KEY)

        /**
         * Removes the suggestion shown in [editor], if any. With [report] the removal is counted
         * as a dismissal; callers that clear on the way to accepting, re-rendering or replacing a
         * suggestion pass false so only real dismissals reach the counters.
         */
        fun clear(editor: Editor, report: Boolean = true) {
            val preview = get(editor) ?: return
            Disposer.dispose(preview)
            if (report) {
                editor.project?.let { FoxxyCodeAutocompleteService.getInstance(it).report("dismissed") }
            }
        }

        /**
         * Replaces whatever is currently shown in [editor] with [text] at [offset]. Must run on the
         * EDT; returns null when there is nothing worth drawing. [report] counts the render as a
         * fresh suggestion shown to the user; a re-render from the prefix cache passes false.
         */
        fun show(editor: Editor, text: String, offset: Int, report: Boolean = true): SuggestionPreview? {
            clear(editor, report = false)
            if (text.isEmpty() || editor.isDisposed || editor.selectionModel.hasSelection()) return null
            if (offset < 0 || offset > editor.document.textLength) return null

            val preview = SuggestionPreview(editor, text, offset)
            EditorUtil.disposeWithEditor(editor, preview)
            editor.putUserData(KEY, preview)
            preview.render()
            if (report) {
                editor.project?.let { FoxxyCodeAutocompleteService.getInstance(it).report("shown") }
            }

            // A caret that leaves the suggestion's anchor invalidates it: the grey text would then
            // describe an insertion point the user is no longer at.
            editor.caretModel.addCaretListener(object : CaretListener {
                override fun caretPositionChanged(event: CaretEvent) {
                    if (editor.caretModel.offset != preview.offset) {
                        Disposer.dispose(preview)
                    }
                }
            }, preview)

            return preview
        }
    }
}
