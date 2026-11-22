package dev.foxxycode.intellij.diff

import com.google.gson.JsonParser

/** The write/edit/apply_patch transforms for a Coddy ToolExecute payload. */
object HookEditTransform {
    fun apply(tool: String, argsJson: String, current: String): String {
        val args = JsonParser.parseString(argsJson).asJsonObject
        return when (tool) {
            "write" -> args.get("content")?.asString ?: throw IllegalArgumentException("write: content is required")
            "edit" -> edit(current, args)
            "apply_patch" -> {
                val patch = args.get("patch")?.asString.orEmpty().ifBlank { args.get("diff")?.asString.orEmpty() }
                if (patch.isBlank()) throw IllegalArgumentException("apply_patch: patch is required")
                patch(current, patch)
            }
            else -> throw IllegalArgumentException("not a content edit: $tool")
        }
    }

    private fun edit(current: String, args: com.google.gson.JsonObject): String {
        var old = args.get("oldString")?.asString.orEmpty()
        var replacement = args.get("newString")?.asString ?: throw IllegalArgumentException("edit: newString is required")
        if (old.isNotEmpty()) {
            val ending = detectLineEnding(current)
            old = convertToLineEnding(old, ending)
            replacement = convertToLineEnding(replacement, ending)
        }
        if (old.isNotEmpty() && old == replacement) throw IllegalArgumentException("edit: oldString and newString must differ")
        if (old.isEmpty()) return replacement
        if (!current.contains(old)) throw IllegalArgumentException("edit: oldString not found in file")
        val replaceAll = args.get("replaceAll")?.takeIf { it.isJsonPrimitive }?.asBoolean ?: false
        return if (replaceAll) current.replace(old, replacement) else current.replaceFirst(old, replacement)
    }

    private fun patch(original: String, diff: String): String =
        if (isV4A(diff)) applyV4A(original, diff) else applyUnified(original, diff)

    private data class Hunk(val oldStart: Int, val oldCount: Int, val newCount: Int)

    private fun applyUnified(original: String, diff: String): String {
        val lines = splitFileLines(normalizeEol(original))
        val diffLines = normalizeEol(diff).split("\n")
        val result = mutableListOf<String>()
        var sourceIndex = 0
        var sawHunk = false
        var i = 0
        while (i < diffLines.size) {
            val line = diffLines[i]
            if (!line.startsWith("@@")) {
                if (!sawHunk && isUnifiedMetadata(line) || line.isEmpty()) {
                    i++
                    continue
                }
                throw IllegalArgumentException("patch line ${quote(line)} outside a hunk")
            }
            val hunk = parseHunk(line)
            val hunkStart = if (hunk.oldStart <= 0) 0 else hunk.oldStart - 1
            if (hunkStart < sourceIndex) throw IllegalArgumentException("patch hunk overlaps a previous hunk")
            if (hunkStart > lines.size) throw IllegalArgumentException("patch hunk starts out of range")
            result.addAll(lines.subList(sourceIndex, hunkStart))
            sourceIndex = hunkStart
            sawHunk = true
            i++
            var oldSeen = 0
            var newSeen = 0
            while (i < diffLines.size && !diffLines[i].startsWith("@@")) {
                val hunkLine = diffLines[i]
                if (hunkLine.isEmpty() || hunkLine.startsWith("\\")) {
                    i++
                    continue
                }
                val text = hunkLine.substring(1)
                when (hunkLine[0]) {
                    ' ' -> {
                        verifySource(lines, sourceIndex, text, "context")
                        result.add(text)
                        sourceIndex++
                        oldSeen++
                        newSeen++
                    }
                    '-' -> {
                        verifySource(lines, sourceIndex, text, "delete")
                        sourceIndex++
                        oldSeen++
                    }
                    '+' -> {
                        result.add(text)
                        newSeen++
                    }
                    else -> throw IllegalArgumentException("patch hunk line ${quote(hunkLine)} has no valid prefix")
                }
                i++
            }
            if (oldSeen != hunk.oldCount || newSeen != hunk.newCount) {
                throw IllegalArgumentException("patch hunk count mismatch: expected -${hunk.oldCount}/+${hunk.newCount}, got -$oldSeen/+$newSeen")
            }
        }
        if (!sawHunk) throw IllegalArgumentException("patch contains no hunks")
        result.addAll(lines.subList(sourceIndex, lines.size))
        return restoreLineEndings(result.joinToString("\n"), original)
    }

    private fun isUnifiedMetadata(line: String): Boolean =
        line.isEmpty() || line.startsWith("diff ") || line.startsWith("index ") ||
            line.startsWith("--- ") || line.startsWith("+++ ")

    private fun verifySource(lines: List<String>, index: Int, expected: String, kind: String) {
        if (index >= lines.size) throw IllegalArgumentException("patch $kind mismatch: expected ${quote(expected)}, found end of file")
        if (lines[index] != expected) {
            throw IllegalArgumentException("patch $kind mismatch at line ${index + 1}: expected ${quote(expected)}, found ${quote(lines[index])}")
        }
    }

    private fun parseHunk(line: String): Hunk {
        val inner = line.removePrefix("@@")
        val end = inner.indexOf("@@")
        if (end < 0) throw IllegalArgumentException("invalid hunk header ${quote(line)}")
        val fields = inner.substring(0, end).trim().split(Regex("\\s+"))
        if (fields.size < 2) throw IllegalArgumentException("invalid hunk header ${quote(line)}")
        val old = parseRange(fields[0], '-')
        val new = parseRange(fields[1], '+')
        return Hunk(old.first, old.second, new.second)
    }

    private fun parseRange(value: String, prefix: Char): Pair<Int, Int> {
        if (value.length < 2 || value[0] != prefix) throw IllegalArgumentException("invalid patch range ${quote(value)}")
        val parts = value.substring(1).split(",", limit = 2)
        val start = parts[0].toInt()
        val count = if (parts.size == 2) parts[1].toInt() else 1
        if (start < 0 || count < 0) throw IllegalArgumentException("negative patch range ${quote(value)}")
        return start to count
    }

    private data class Chunk(
        val context: String = "",
        val oldLines: MutableList<String> = mutableListOf(),
        val newLines: MutableList<String> = mutableListOf(),
        var eof: Boolean = false,
    )

    private data class Replacement(val start: Int, val oldLength: Int, val newLines: List<String>)

    private fun isV4A(diff: String): Boolean {
        if (diff.contains("*** Begin Patch")) return true
        return diff.split("\n").any { line ->
            val trimmed = line.trim()
            if (!trimmed.startsWith("@@")) return@any false
            val inner = trimmed.removePrefix("@@").trim().removeSuffix("@@").trim()
            if (inner.isEmpty()) return@any true
            if (inner.startsWith("-")) {
                val start = inner.substringBefore(" ").removePrefix("-").substringBefore(",")
                if (start.toIntOrNull() != null) return@any false
            }
            true
        }
    }

    private fun applyV4A(original: String, diff: String): String {
        val chunks = parseV4A(normalizeEol(diff))
        if (chunks.isEmpty()) throw IllegalArgumentException("patch contains no change hunks")
        val lines = splitFileLines(normalizeEol(original))
        val replacements = mutableListOf<Replacement>()
        var lineIndex = 0
        for (chunk in chunks) {
            if (chunk.context.isNotEmpty()) {
                val found = seek(lines, listOf(chunk.context), lineIndex, false)
                    ?: throw IllegalArgumentException("patch context not found: ${quote(chunk.context)}")
                lineIndex = found + 1
            }
            if (chunk.oldLines.isEmpty()) {
                val insertAt = if (lines.isNotEmpty() && lines.last().isEmpty()) lines.size - 1 else lines.size
                replacements.add(Replacement(insertAt, 0, chunk.newLines.toList()))
                continue
            }
            var pattern = chunk.oldLines.toList()
            var newLines = chunk.newLines.toList()
            var found = seek(lines, pattern, lineIndex, chunk.eof)
            if (found == null && pattern.lastOrNull()?.isEmpty() == true) {
                pattern = pattern.dropLast(1)
                if (newLines.lastOrNull()?.isEmpty() == true) newLines = newLines.dropLast(1)
                found = seek(lines, pattern, lineIndex, chunk.eof)
            }
            val start = found ?: throw IllegalArgumentException("patch context not found in file:\n${chunk.oldLines.joinToString("\n")}")
            replacements.add(Replacement(start, pattern.size, newLines))
            lineIndex = start + pattern.size
        }
        val result = lines.toMutableList()
        for (replacement in replacements.asReversed()) {
            result.subList(replacement.start, replacement.start + replacement.oldLength).clear()
            result.addAll(replacement.start, replacement.newLines)
        }
        return restoreLineEndings(result.joinToString("\n"), original)
    }

    private fun parseV4A(diff: String): List<Chunk> {
        val chunks = mutableListOf<Chunk>()
        var current: Chunk? = null
        fun flush() {
            val c = current ?: return
            if (c.oldLines.isNotEmpty() || c.newLines.isNotEmpty()) chunks.add(c)
            current = null
        }
        for (line in diff.split("\n")) {
            val trimmed = line.trim()
            when {
                trimmed == "*** Begin Patch" || trimmed == "*** End Patch" -> continue
                trimmed.startsWith("*** Add File:") || trimmed.startsWith("*** Delete File:") ||
                    trimmed.startsWith("*** Update File:") || trimmed.startsWith("*** Move to:") ||
                    trimmed.startsWith("*** Environment ID:") -> continue
                // Unified-diff metadata headers carry a space ("--- <path>"); a v4A
                // deletion of a line starting with "--" is "---..." with none, and
                // must reach the hunk parser (the agent's own patch.go does the same).
                trimmed.startsWith("--- ") || trimmed.startsWith("+++ ") -> continue
                trimmed == "@@" || trimmed.startsWith("@@ ") -> {
                    flush()
                    current = Chunk(context = if (trimmed == "@@") "" else trimmed.removePrefix("@@").trim())
                    continue
                }
                trimmed == "*** End of File" -> {
                    val c = current ?: throw IllegalArgumentException("patch: End of File without a preceding hunk")
                    c.eof = true
                    continue
                }
            }
            if (current == null) {
                if (line.isEmpty()) continue
                if (line[0] != ' ' && line[0] != '+' && line[0] != '-') continue
                current = Chunk()
            }
            val c = current!!
            when {
                line.isEmpty() -> {
                    c.oldLines.add("")
                    c.newLines.add("")
                }
                line[0] == ' ' -> {
                    c.oldLines.add(line.substring(1))
                    c.newLines.add(line.substring(1))
                }
                line[0] == '+' -> c.newLines.add(line.substring(1))
                line[0] == '-' -> c.oldLines.add(line.substring(1))
                else -> throw IllegalArgumentException("patch hunk line ${quote(line)} has no valid prefix")
            }
        }
        flush()
        return chunks
    }

    private fun seek(lines: List<String>, pattern: List<String>, start: Int, eof: Boolean): Int? {
        if (pattern.isEmpty()) return start
        if (pattern.size > lines.size) return null
        val searchStart = if (eof && lines.size >= pattern.size) lines.size - pattern.size else start
        val comparators: List<(String, String) -> Boolean> = listOf(
            { a, b -> a == b },
            { a, b -> a.trimEnd(' ', '\t') == b.trimEnd(' ', '\t') },
            { a, b -> a.trim() == b.trim() },
        )
        for (equal in comparators) {
            for (i in searchStart..lines.size - pattern.size) {
                if (pattern.indices.all { j -> equal(lines[i + j], pattern[j]) }) return i
            }
        }
        return null
    }

    private fun splitFileLines(text: String): List<String> {
        val lines = text.split("\n")
        return if (lines.isNotEmpty() && lines.last().isEmpty()) lines.dropLast(1) else lines
    }

    private fun detectLineEnding(text: String): String {
        for (i in text.indices) {
            when (text[i]) {
                '\n' -> return "\n"
                '\r' -> return if (i + 1 < text.length && text[i + 1] == '\n') "\r\n" else "\r"
            }
        }
        return "\n"
    }

    private fun normalizeEol(text: String): String = text.replace("\r\n", "\n").replace("\r", "\n")

    private fun convertToLineEnding(text: String, ending: String): String {
        val normalized = normalizeEol(text)
        return if (ending == "\n") normalized else normalized.replace("\n", ending)
    }

    private fun restoreLineEndings(text: String, source: String): String {
        var normalized = normalizeEol(text)
        if (source.endsWith("\n") || source.endsWith("\r")) {
            if (normalized.isNotEmpty()) normalized += "\n"
        } else {
            normalized = normalized.removeSuffix("\n")
        }
        return convertToLineEnding(normalized, detectLineEnding(source))
    }

    private fun quote(text: String): String = "\"" + text.replace("\"", "\\\"") + "\""
}
