package dev.foxxy.intellij.ui

import com.intellij.ui.ColorUtil
import com.intellij.util.ui.UIUtil

/**
 * Bridges the IDE look-and-feel to the foxxy web UI theme.
 *
 * The foxxy SPA exposes `window.foxxyUi` (see docs/intellij-embedding.md):
 *   - `setTheme(id)` applies + persists the theme (id: light|dark|midnight|solarized-dark|…)
 *   - `?theme=<id>` on the initial URL applies the theme before first paint (no flash)
 *
 * Only `light` is a light theme; every other id is dark. We map the IDE LAF to
 * `light` / `dark` accordingly. See `foxxyUiApi.ts` for the full contract.
 */
object FoxxyThemeBridge {

    /** foxxy theme id matching the current IDE LAF. */
    fun currentFoxxyTheme(): String =
        if (ColorUtil.isDark(UIUtil.getPanelBackground())) "dark" else "light"

    /** JS that live-applies the theme via the foxxyUi API (mirrors window.foxxyUi.setTheme). */
    fun applyThemeJs(mode: String): String {
        val m = mode.replace("\"", "")
        return "(function(){ try { if (window.foxxyUi) window.foxxyUi.setTheme(\"$m\"); } catch (e) {} })();"
    }
}
