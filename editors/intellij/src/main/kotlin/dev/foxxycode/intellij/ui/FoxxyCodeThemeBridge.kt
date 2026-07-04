package dev.foxxycode.intellij.ui

import com.intellij.ui.ColorUtil
import com.intellij.util.ui.UIUtil

/**
 * Bridges the IDE look-and-feel to the foxxycode web UI theme.
 *
 * The foxxycode SPA exposes `window.foxxycodeUi` (see docs/intellij-embedding.md):
 *   - `setTheme(id)` applies + persists the theme (id: light|dark|midnight|solarized-dark|…)
 *   - `?theme=<id>` on the initial URL applies the theme before first paint (no flash)
 *
 * Only `light` is a light theme; every other id is dark. We map the IDE LAF to
 * `light` / `dark` accordingly. See `foxxycodeUiApi.ts` for the full contract.
 */
object FoxxyCodeThemeBridge {

    /** foxxycode theme id matching the current IDE LAF. */
    fun currentFoxxyCodeTheme(): String =
        if (ColorUtil.isDark(UIUtil.getPanelBackground())) "dark" else "light"

    /** JS that live-applies the theme via the foxxycodeUi API (mirrors window.foxxycodeUi.setTheme). */
    fun applyThemeJs(mode: String): String {
        val m = mode.replace("\"", "")
        return "(function(){ try { if (window.foxxycodeUi) window.foxxycodeUi.setTheme(\"$m\"); } catch (e) {} })();"
    }
}
