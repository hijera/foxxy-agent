package dev.foxxycode.intellij.diff

internal data class ReviewHunk(val start: Int, val end: Int, val before: String, val after: String) {
    fun matches(text: CharSequence): Boolean =
        start >= 0 && end <= text.length && text.subSequence(start, end).toString() == after

    fun overlaps(other: ReviewHunk): Boolean =
        if (start == end || other.start == other.end) start <= other.end && other.start <= end
        else start < other.end && other.start < end

    fun move(changeStart: Int, changeEnd: Int, newLength: Int): ReviewHunk? = when {
        start == end && changeStart == start -> null
        changeEnd <= start -> {
            val delta = newLength - (changeEnd - changeStart)
            copy(start = start + delta, end = end + delta)
        }
        changeStart >= end -> this
        else -> null
    }
}

internal fun editorReviewText(text: String): String =
    text.removePrefix("\uFEFF").replace("\r\n", "\n").replace('\r', '\n')

/** Offsets include line separators; a deleted block is an empty range in the applied text. */
internal fun reviewHunks(before: String, after: String): List<ReviewHunk> {
    fun lines(text: String) = Regex("[^\\n]*\\n|[^\\n]+$").findAll(text).map { it.value }.toList()
    fun offsets(lines: List<String>) = IntArray(lines.size + 1).also { offsets ->
        for (index in lines.indices) offsets[index + 1] = offsets[index] + lines[index].length
    }
    val oldLines = lines(before)
    val newLines = lines(after)
    val oldOffsets = offsets(oldLines)
    val newOffsets = offsets(newLines)
    return ChangedLineRanges.fragments(oldLines, newLines).map { fragment ->
        val start = newOffsets[fragment.start2]
        val end = newOffsets[fragment.end2]
        ReviewHunk(start, end, before.substring(oldOffsets[fragment.start1], oldOffsets[fragment.end1]), after.substring(start, end))
    }
}
