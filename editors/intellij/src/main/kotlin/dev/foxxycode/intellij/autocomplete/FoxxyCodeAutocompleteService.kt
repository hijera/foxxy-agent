package dev.foxxycode.intellij.autocomplete

import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.application.ReadAction
import com.intellij.openapi.diagnostic.logger
import com.intellij.openapi.editor.Editor
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.project.Project
import com.intellij.util.Alarm
import dev.foxxycode.intellij.process.FoxxyCodeProcessManager
import kotlin.math.max
import kotlin.math.min

/**
 * Drives inline code completion for one project: keeps the backend's client settings, debounces
 * typing, and owns the single in-flight request.
 *
 * The shape is taken from tabnine's inline handler, minus its sidecar protocol. Three of its ideas
 * carry the feature and are implemented here:
 *
 *  - **the prefix cache** ([SuggestionText.advance]) - typing the characters a suggestion already
 *    predicted re-renders locally instead of asking again. Without it an LLM-backed completion
 *    feels dead, because every keystroke would wait on a round trip;
 *  - **one in-flight request** - a newer keystroke cancels the older call, which drops the socket
 *    and kills the upstream LLM call with it;
 *  - **a modification-stamp guard** - an answer that arrives after the document or the caret moved
 *    is discarded rather than drawn into text it no longer describes.
 *
 * Registered as a projectService; [startIfNeeded] is called once the backend is up, like the other
 * IDE-context services.
 */
class FoxxyCodeAutocompleteService(private val project: Project) : Disposable {
    private val log = logger<FoxxyCodeAutocompleteService>()

    private val typingAlarm = Alarm(Alarm.ThreadToUse.POOLED_THREAD, this)
    private val configAlarm = Alarm(Alarm.ThreadToUse.POOLED_THREAD, this)

    @Volatile
    private var clientConfig: AutocompleteClientConfig = AutocompleteClientConfig.DISABLED

    @Volatile
    private var inFlight: CompletionClient.InFlight? = null

    private var configPollStarted = false

    /** The settings the backend published; [AutocompleteClientConfig.DISABLED] until it answers. */
    val config: AutocompleteClientConfig get() = clientConfig

    /** Starts polling the backend for the autocomplete settings (idempotent). */
    fun startIfNeeded() {
        if (!configPollStarted) {
            configPollStarted = true
        }
        scheduleConfigRefresh(0)
    }

    override fun dispose() {
        typingAlarm.cancelAllRequests()
        configAlarm.cancelAllRequests()
        inFlight?.cancel()
    }

    private fun scheduleConfigRefresh(delayMs: Int) {
        if (project.isDisposed) return
        configAlarm.cancelAllRequests()
        configAlarm.addRequest({ refreshConfig() }, delayMs)
    }

    private fun refreshConfig() {
        if (project.isDisposed) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl
        if (base != null) {
            CompletionClient.fetchConfig(base)?.let { fetched ->
                if (fetched != clientConfig) {
                    log.info("autocomplete config: enabled=${fetched.enabled} trigger=${fetched.trigger}")
                }
                clientConfig = fetched
            }
        }
        scheduleConfigRefresh(CONFIG_POLL_MS)
    }

    /**
     * Called for every ordinary typing change. Runs on the EDT inside the document change, so it
     * only captures state and hands the work on. [lineBefore] is the caret's line up to the caret
     * and [charAfter] the character right after it, or null at the end of the document.
     */
    fun onTyped(editor: Editor, offset: Int, typed: String, lineBefore: String, charAfter: Char?) {
        val cfg = clientConfig
        if (!cfg.enabled) return

        val shown = SuggestionPreview.get(editor)
        val advanced = shown
            ?.takeIf { it.offset + typed.length == offset }
            ?.let { SuggestionText.advance(it.text, typed) }
        SuggestionPreview.clear(editor, report = false)

        // The user typed exactly what was suggested, to the end: nothing is left to draw, and
        // asking again immediately would just bill for a suggestion nobody asked for.
        if (advanced != null && advanced.isEmpty()) {
            cancelPending()
            return
        }
        if (advanced != null) {
            cancelPending()
            report("cache_hit")
            ApplicationManager.getApplication().invokeLater {
                if (!editor.isDisposed && editor.caretModel.offset == offset) {
                    SuggestionPreview.show(editor, advanced, offset, report = false)
                }
            }
            return
        }

        if (!cfg.automatic || !SuggestionText.shouldRequest(lineBefore, typed, charAfter)) return
        // The provider rate-limited us a moment ago: automatic requests wait it out (the manual
        // shortcut still works, because then the user has asked).
        if (CompletionClient.isPaused()) return
        scheduleFetch(editor, offset, cfg.debounceMs)
    }

    /**
     * Posts one outcome (shown, accepted, dismissed, cache_hit) to the backend's counters.
     * Fire-and-forget: the numbers are what decide whether the feature is worth keeping, and
     * nothing in the editor waits for them.
     */
    fun report(event: String) {
        if (project.isDisposed) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        ApplicationManager.getApplication().executeOnPooledThread {
            CompletionClient.sendFeedback(base, event)
        }
    }

    /** Asks for a suggestion right now, ignoring the trigger mode. Backs the editor shortcut. */
    fun triggerManually(editor: Editor) {
        if (!clientConfig.enabled) return
        SuggestionPreview.clear(editor, report = false)
        scheduleFetch(editor, editor.caretModel.offset, 0)
    }

    private fun cancelPending() {
        typingAlarm.cancelAllRequests()
        inFlight?.cancel()
    }

    private fun scheduleFetch(editor: Editor, offset: Int, delayMs: Int) {
        cancelPending()
        if (project.isDisposed) return
        typingAlarm.addRequest({ fetch(editor, offset) }, delayMs)
    }

    private fun fetch(editor: Editor, offset: Int) {
        if (project.isDisposed || editor.isDisposed) return
        val base = FoxxyCodeProcessManager.getInstance(project).baseUrl ?: return
        val cfg = clientConfig

        val snapshot = ReadAction.compute<Snapshot?, RuntimeException> { snapshot(editor, offset, cfg) } ?: return

        val flight = CompletionClient.InFlight()
        inFlight = flight
        val text = CompletionClient.fetchCompletion(base, snapshot.request, cfg.timeoutMs, flight) ?: return
        if (flight.cancelled) return

        ApplicationManager.getApplication().invokeLater {
            if (editor.isDisposed || flight.cancelled) return@invokeLater
            // The answer describes the document as it was when we asked. If either the text or the
            // caret moved on, drawing it would put grey text where it does not belong.
            if (editor.document.modificationStamp != snapshot.modificationStamp) return@invokeLater
            if (editor.caretModel.offset != offset) return@invokeLater
            SuggestionPreview.show(editor, text, offset)
        }
    }

    private class Snapshot(val request: CompletionClient.Request, val modificationStamp: Long)

    private fun snapshot(editor: Editor, offset: Int, cfg: AutocompleteClientConfig): Snapshot? {
        if (editor.isDisposed || project.isDisposed) return null
        val document = editor.document
        if (offset < 0 || offset > document.textLength) return null

        val text = document.charsSequence
        val from = max(0, offset - cfg.maxPrefixBytes)
        val to = min(document.textLength, offset + cfg.maxSuffixBytes)
        val prefix = text.subSequence(from, offset).toString()
        val suffix = text.subSequence(offset, to).toString()
        if (prefix.isBlank() && suffix.isBlank()) return null

        val file = FileDocumentManager.getInstance().getFile(document)
        return Snapshot(
            CompletionClient.Request(
                prefix = prefix,
                suffix = suffix,
                path = file?.path.orEmpty(),
                language = file?.fileType?.name?.lowercase().orEmpty(),
            ),
            document.modificationStamp,
        )
    }

    companion object {
        private const val CONFIG_POLL_MS = 30_000

        fun getInstance(project: Project): FoxxyCodeAutocompleteService =
            project.getService(FoxxyCodeAutocompleteService::class.java)
    }
}
