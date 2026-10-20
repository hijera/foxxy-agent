package dev.foxxycode.intellij.diff

/**
 * Which lines of an editor document an agent edit highlights. Kept free of IntelliJ types so it
 * can be unit-tested without an IDE fixture, which is the only kind of test this plugin runs.
 */
object ChangedLineRanges {

    /** Which text of the event the highlighted document is supposed to show. */
    enum class Side { BEFORE, AFTER }

    /** What happened to a block, which picks the diff color it is painted with. */
    enum class Kind { INSERTED, DELETED, MODIFIED }

    /** Document lines [start] until [end] (0-based, end-exclusive). */
    data class LineRange(val start: Int, val end: Int, val kind: Kind)

    /** Lines [start1] until [end1] of the old text became [start2] until [end2] of the new one. */
    internal data class Fragment(val start1: Int, val end1: Int, val start2: Int, val end2: Int)

    /**
     * Beyond this many inserted plus deleted lines the diff stops looking for the shortest edit
     * and reports everything between the common head and tail as one block. Keeps a whole-file
     * rewrite from costing quadratic memory on the EDT.
     */
    private const val MAX_EDIT_DISTANCE = 1000

    /**
     * The lines of the [before] -> [after] change as they appear in [document] — the editor's own
     * text, which is supposed to show [side] of the change.
     *
     * Line numbers from the event are never used on the document directly. The document can lag
     * behind the disk (an open file the VFS has not reloaded yet still holds the pre-edit text),
     * or be ahead of the event (a second edit already landed), and a highlighter placed by stale
     * numbers sits on the wrong lines and is then carried further off by the reload. Instead each
     * changed line is paired with its equal line in the document through a second diff; a changed
     * line the document does not contain is not highlighted at all.
     */
    fun inDocument(before: String, after: String, document: CharSequence, side: Side): List<LineRange> {
        val oldLines = lines(before)
        val newLines = lines(after)
        val shown = if (side == Side.AFTER) newLines else oldLines
        val toDocument = matchLines(shown.map(::alignKey), lines(document).map(::alignKey))

        val result = ArrayList<LineRange>()
        for (f in fragments(oldLines, newLines)) {
            val start = if (side == Side.AFTER) f.start2 else f.start1
            val end = if (side == Side.AFTER) f.end2 else f.end1
            if (end <= start) continue // a pure deletion has no line on the after side, and vice versa
            val kind = when {
                f.end1 == f.start1 -> Kind.INSERTED
                f.end2 == f.start2 -> Kind.DELETED
                else -> Kind.MODIFIED
            }
            var runStart = -1
            var runEnd = -1
            for (line in start until end) {
                val at = toDocument[line]
                if (at >= 0 && at == runEnd) {
                    runEnd++
                    continue
                }
                if (runStart >= 0) result.add(LineRange(runStart, runEnd, kind))
                runStart = at
                runEnd = if (at >= 0) at + 1 else -1
            }
            if (runStart >= 0) result.add(LineRange(runStart, runEnd, kind))
        }
        return result
    }

    /**
     * A line as compared against the document. The server sends raw file bytes as a JSON string,
     * so non-UTF-8 text (a cp1251 source) arrives with every non-ASCII run mangled into U+FFFD or
     * into other letters — and those lines would never equal the document's properly decoded
     * ones. Collapsing each run of non-ASCII characters to one placeholder lets them pair up.
     */
    private fun alignKey(line: String): String {
        if (line.all { it.code < 0x80 }) return line
        val sb = StringBuilder(line.length)
        var inRun = false
        for (c in line) {
            if (c.code < 0x80) {
                sb.append(c)
                inRun = false
            } else if (!inRun) {
                sb.append('�')
                inRun = true
            }
        }
        return sb.toString()
    }

    /**
     * Splits text the way an IntelliJ document counts lines — `\r\n`, `\r` and `\n` all end one —
     * except that the empty remainder after a final line break is not a line of its own. A
     * leading byte order mark is dropped: the document never contains it.
     */
    internal fun lines(text: CharSequence): List<String> {
        val start = if (text.isNotEmpty() && text[0] == '﻿') 1 else 0
        val out = ArrayList<String>()
        var lineStart = start
        var i = start
        while (i < text.length) {
            val c = text[i]
            if (c == '\n' || c == '\r') {
                out.add(text.subSequence(lineStart, i).toString())
                if (c == '\r' && i + 1 < text.length && text[i + 1] == '\n') i++
                lineStart = i + 1
            }
            i++
        }
        if (lineStart < text.length) out.add(text.subSequence(lineStart, text.length).toString())
        return out
    }

    /** The blocks that differ between [a] and [b], in order, from a shortest line edit script. */
    internal fun fragments(a: List<String>, b: List<String>, maxEditDistance: Int = MAX_EDIT_DISTANCE): List<Fragment> {
        val match = matchLines(a, b, maxEditDistance)
        val out = ArrayList<Fragment>()
        var i = 0
        var j = 0
        while (i < a.size || j < b.size) {
            if (i < a.size && match[i] == j) {
                i++
                j++
                continue
            }
            var i2 = i
            while (i2 < a.size && match[i2] < 0) i2++
            val j2 = if (i2 < a.size) match[i2] else b.size
            out.add(Fragment(i, i2, j, j2))
            i = i2
            j = j2
        }
        return out
    }

    /**
     * For every line of [a], the index of the equal line of [b] it is paired with, or -1 when it
     * was deleted. Pairs are increasing on both sides and as many as possible (Myers' O(ND)
     * algorithm after trimming the common head and tail).
     */
    internal fun matchLines(a: List<String>, b: List<String>, maxEditDistance: Int = MAX_EDIT_DISTANCE): IntArray {
        val match = IntArray(a.size) { -1 }
        // Compare interned ids instead of strings: the inner loop runs once per diagonal step.
        val ids = HashMap<String, Int>()
        val x = IntArray(a.size) { ids.getOrPut(a[it]) { ids.size } }
        val y = IntArray(b.size) { ids.getOrPut(b[it]) { ids.size } }

        var head = 0
        while (head < x.size && head < y.size && x[head] == y[head]) {
            match[head] = head
            head++
        }
        var tail = 0
        while (tail < x.size - head && tail < y.size - head && x[x.size - 1 - tail] == y[y.size - 1 - tail]) {
            match[x.size - 1 - tail] = y.size - 1 - tail
            tail++
        }
        val n = x.size - head - tail
        val m = y.size - head - tail
        if (n == 0 || m == 0) return match

        val limit = minOf(n + m, maxEditDistance)
        // v[k + offset] = the furthest x reached on diagonal k; trace[d] keeps v[-d-1..d+1] as it
        // was before step d, which is all the backtrack needs.
        val offset = limit + 1
        val v = IntArray(2 * limit + 3)
        val trace = ArrayList<IntArray>()
        for (d in 0..limit) {
            trace.add(v.copyOfRange(offset - d - 1, offset + d + 2))
            var k = -d
            while (k <= d) {
                var px = if (k == -d || (k != d && v[offset + k - 1] < v[offset + k + 1])) {
                    v[offset + k + 1]
                } else {
                    v[offset + k - 1] + 1
                }
                var py = px - k
                while (px < n && py < m && x[head + px] == y[head + py]) {
                    px++
                    py++
                }
                v[offset + k] = px
                if (px >= n && py >= m) {
                    backtrack(trace, n, m, head, match)
                    return match
                }
                k += 2
            }
        }
        return match // too far apart: only the common head and tail stay paired
    }

    private fun backtrack(trace: List<IntArray>, n: Int, m: Int, head: Int, match: IntArray) {
        var px = n
        var py = m
        for (d in trace.indices.reversed()) {
            val v = trace[d] // v[k] is at index k + d + 1
            val k = px - py
            val prevK = if (k == -d || (k != d && v[k - 1 + d + 1] < v[k + 1 + d + 1])) k + 1 else k - 1
            val prevX = if (d == 0) 0 else v[prevK + d + 1]
            val prevY = prevX - prevK
            while (px > prevX && py > prevY) {
                px--
                py--
                match[head + px] = head + py
            }
            px = prevX
            py = prevY
        }
    }
}
