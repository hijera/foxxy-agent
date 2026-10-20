package dev.foxxycode.intellij.diff

import dev.foxxycode.intellij.diff.ChangedLineRanges.Kind
import dev.foxxycode.intellij.diff.ChangedLineRanges.LineRange
import dev.foxxycode.intellij.diff.ChangedLineRanges.Side
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.random.Random

/**
 * Unit tests for [ChangedLineRanges] — which lines of the *editor document* an agent edit
 * highlights. Platform-free, so it runs in the plain JUnit4 `test` task.
 *
 * The fixture is the edit from the reported bug: one `edit` call that adds two imports and a
 * twelve-line block under the CreateTodo header of a 64-line Go file.
 */
class ChangedLineRangesTest {

    private val before = """
        package main

        import (
        	"encoding/json"
        	"net/http"
        	"sync"
        )

        type Todo struct {
        	ID    int    `json:"id"`
        	Title string `json:"title"`
        	Done  bool   `json:"done"`
        }

        type Store struct {
        	mu    sync.Mutex
        	next  int
        	items map[int]Todo
        }

        func NewStore() *Store {
        	return &Store{next: 1, items: map[int]Todo{}}
        }

        func (s *Store) List(w http.ResponseWriter, r *http.Request) {
        	s.mu.Lock()
        	defer s.mu.Unlock()
        	out := make([]Todo, 0, len(s.items))
        	for _, t := range s.items {
        		out = append(out, t)
        	}
        	json.NewEncoder(w).Encode(out)
        }

        func (s *Store) CreateTodo(w http.ResponseWriter, r *http.Request) {
        	var t Todo
        	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
        		http.Error(w, err.Error(), http.StatusBadRequest)
        		return
        	}
        	s.mu.Lock()
        	t.ID = s.next
        	s.next++
        	s.items[t.ID] = t
        	s.mu.Unlock()
        	w.WriteHeader(http.StatusCreated)
        	json.NewEncoder(w).Encode(t)
        }

        func (s *Store) Delete(w http.ResponseWriter, r *http.Request) {
        	id, err := strconv.Atoi(r.URL.Query().Get("id"))
        	if err != nil {
        		http.Error(w, "bad id", http.StatusBadRequest)
        		return
        	}
        	s.mu.Lock()
        	delete(s.items, id)
        	s.mu.Unlock()
        	w.WriteHeader(http.StatusNoContent)
        }

        func main() {
        	http.ListenAndServe(":8080", nil)
        }
    """.trimIndent() + "\n"

    private val importLines = listOf("\t\"strconv\"", "\t\"strings\"")

    private val validation = listOf(
        "\tif r.Method != http.MethodPost {",
        "\t\thttp.Error(w, \"method not allowed\", http.StatusMethodNotAllowed)",
        "\t\treturn",
        "\t}",
        "\tt.Title = strings.TrimSpace(t.Title)",
        "\tif t.Title == \"\" {",
        "\t\thttp.Error(w, \"title is required\", http.StatusBadRequest)",
        "\t\treturn",
        "\t}",
        "\tif len(t.Title) > 200 {",
        "\t\thttp.Error(w, \"title is too long\", http.StatusBadRequest)",
        "\t\treturn",
    )

    /** The pre-edit file with lines inserted after the given 0-based line indexes. */
    private fun insertAfter(text: String, vararg inserts: Pair<Int, List<String>>): String {
        val lines = text.split("\n").toMutableList()
        for ((index, added) in inserts.sortedByDescending { it.first }) {
            lines.addAll(index + 1, added)
        }
        return lines.joinToString("\n")
    }

    // `)` of the import block is line 6 (0-based) before the edit; the CreateTodo header is 34.
    private val after = insertAfter(before, 5 to importLines, 34 to validation)

    @Test
    fun `fixture has the reported shape`() {
        assertEquals(65, before.split("\n").size) // 64 lines plus the empty one after the last \n
        assertEquals(79, after.split("\n").size)
    }

    @Test
    fun `a document that already holds the edit highlights exactly the inserted lines`() {
        assertEquals(
            listOf(LineRange(6, 8, Kind.INSERTED), LineRange(37, 49, Kind.INSERTED)),
            ChangedLineRanges.inDocument(before, after, after, Side.AFTER),
        )
    }

    @Test
    fun `a document still holding the pre-edit text gets no highlight`() {
        // The bug: the editor had the file open, the VFS had not reloaded it yet, and the
        // after-edit line numbers 37..49 were painted over the old CreateTodo body — then
        // carried below the insertion when the reload finally arrived.
        assertEquals(
            emptyList<LineRange>(),
            ChangedLineRanges.inDocument(before, after, before, Side.AFTER),
        )
    }

    @Test
    fun `lines added above the edit since it was made shift the highlight with them`() {
        val document = "// header 1\n// header 2\n// header 3\n$after"
        assertEquals(
            listOf(LineRange(9, 11, Kind.INSERTED), LineRange(40, 52, Kind.INSERTED)),
            ChangedLineRanges.inDocument(before, after, document, Side.AFTER),
        )
    }

    @Test
    fun `a later edit inside the highlighted block splits it instead of shifting it`() {
        // Two edits to one file: the second replaced one line of the first edit's block.
        val lines = after.split("\n").toMutableList()
        lines[42] = "\tlog.Printf(\"create todo\")"
        assertEquals(
            listOf(
                LineRange(6, 8, Kind.INSERTED),
                LineRange(37, 42, Kind.INSERTED),
                LineRange(43, 49, Kind.INSERTED),
            ),
            ChangedLineRanges.inDocument(before, after, lines.joinToString("\n"), Side.AFTER),
        )
    }

    @Test
    fun `a proposed edit marks the lines it will replace in the untouched document`() {
        val proposed = before.replace("\tjson.NewEncoder(w).Encode(t)\n", "\t_ = json.NewEncoder(w).Encode(t)\n")
        assertEquals(
            listOf(LineRange(46, 47, Kind.MODIFIED)),
            ChangedLineRanges.inDocument(before, proposed, before, Side.BEFORE),
        )
    }

    @Test
    fun `a proposed deletion is marked as deleted on the pre-edit side`() {
        val proposed = before.replace("\ts.next++\n", "")
        assertEquals(
            listOf(LineRange(42, 43, Kind.DELETED)),
            ChangedLineRanges.inDocument(before, proposed, before, Side.BEFORE),
        )
        // ...and has no line of its own to paint once applied.
        assertEquals(emptyList<LineRange>(), ChangedLineRanges.inDocument(before, proposed, proposed, Side.AFTER))
    }

    @Test
    fun `CRLF event content lines up with the editor's LF document`() {
        val crlfBefore = before.replace("\n", "\r\n")
        val crlfAfter = after.replace("\n", "\r\n")
        assertEquals(
            listOf(LineRange(6, 8, Kind.INSERTED), LineRange(37, 49, Kind.INSERTED)),
            ChangedLineRanges.inDocument(crlfBefore, crlfAfter, after, Side.AFTER),
        )
    }

    @Test
    fun `a byte order mark in the event content does not unmatch the first line`() {
        assertEquals(
            listOf(LineRange(0, 1, Kind.MODIFIED)),
            ChangedLineRanges.inDocument("﻿one\ntwo\n", "﻿ONE\ntwo\n", "ONE\ntwo\n", Side.AFTER),
        )
    }

    @Test
    fun `cp1251 text that reached the IDE as replacement characters still lines up`() {
        // The server sends raw file bytes as a JSON string, so a cp1251 Cyrillic word arrives as
        // U+FFFD per byte — or, where two bytes happen to form valid UTF-8, as a different letter.
        val eventBefore = "// �����\nx := 1\n"
        val eventAfter = "// �����\nx := 2\ny := 3\n"
        val document = "// Привет\nx := 2\ny := 3\n"
        assertEquals(
            listOf(LineRange(1, 3, Kind.MODIFIED)),
            ChangedLineRanges.inDocument(eventBefore, eventAfter, document, Side.AFTER),
        )
    }

    @Test
    fun `a changed line whose text is not in the document is not highlighted`() {
        assertEquals(
            listOf(LineRange(2, 3, Kind.MODIFIED)),
            ChangedLineRanges.inDocument("a\nb\nc\n", "a\nB\nC\n", "a\nb\nC\n", Side.AFTER),
        )
    }

    @Test
    fun `identical texts have nothing to highlight`() {
        assertEquals(emptyList<LineRange>(), ChangedLineRanges.inDocument(before, before, before, Side.AFTER))
        assertEquals(emptyList<LineRange>(), ChangedLineRanges.inDocument("", "", "", Side.AFTER))
    }

    @Test
    fun `a new file is one inserted block`() {
        assertEquals(
            listOf(LineRange(0, 2, Kind.INSERTED)),
            ChangedLineRanges.inDocument("", "one\ntwo\n", "one\ntwo\n", Side.AFTER),
        )
    }

    @Test
    fun `fragments rebuild the new text and are minimal`() {
        val random = Random(20260915)
        repeat(400) {
            val a = List(random.nextInt(0, 14)) { "l${random.nextInt(0, 4)}" }
            val b = List(random.nextInt(0, 14)) { "l${random.nextInt(0, 4)}" }
            val fragments = ChangedLineRanges.fragments(a, b)

            val rebuilt = ArrayList<String>()
            var i = 0
            for (f in fragments) {
                assertTrue("fragments out of order: $fragments", f.start1 >= i)
                rebuilt.addAll(a.subList(i, f.start1))
                rebuilt.addAll(b.subList(f.start2, f.end2))
                i = f.end1
            }
            rebuilt.addAll(a.subList(i, a.size))
            assertEquals("$a -> $b", b, rebuilt)

            val unchanged = a.size - fragments.sumOf { it.end1 - it.start1 }
            assertEquals("$a -> $b is not a longest common subsequence", lcs(a, b), unchanged)
        }
    }

    @Test
    fun `an edit too large to diff line by line falls back to one covering fragment`() {
        val a = List(50) { "a$it" }
        val b = listOf("keep") + List(50) { "b$it" } + listOf("tail")
        val fragments = ChangedLineRanges.fragments(listOf("keep") + a + listOf("tail"), b, maxEditDistance = 10)
        assertEquals(listOf(ChangedLineRanges.Fragment(1, 51, 1, 51)), fragments)
    }

    private fun lcs(a: List<String>, b: List<String>): Int {
        val dp = Array(a.size + 1) { IntArray(b.size + 1) }
        for (i in a.indices.reversed()) for (j in b.indices.reversed()) {
            dp[i][j] = if (a[i] == b[j]) dp[i + 1][j + 1] + 1 else maxOf(dp[i + 1][j], dp[i][j + 1])
        }
        return dp[0][0]
    }
}
