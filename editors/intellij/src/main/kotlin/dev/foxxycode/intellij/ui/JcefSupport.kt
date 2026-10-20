package dev.foxxycode.intellij.ui

/**
 * Answers whether this plugin can load the embedded browser classes at all.
 *
 * From build 262 (IDE 2026.2) `com.intellij.ui.jcef.*` ships in the bundled "Web Browser (JCEF)"
 * plugin instead of the platform core, and only plugins that depend on it see its classes. The
 * optional `com.intellij.modules.jcef` dependency in plugin.xml provides that; this check covers
 * what it cannot: the JCEF plugin disabled by the user, or an IDE that ships it under another id.
 *
 * Nothing here may mention a JCEF type in a signature or a field. Code that does is linked when its
 * class is loaded, so the check would fail with NoClassDefFoundError before it could answer, which
 * is exactly how the tool window used to stay empty.
 */
object JcefSupport {
    private val requiredClasses = listOf(
        "com.intellij.ui.jcef.JBCefApp",
        "com.intellij.ui.jcef.JBCefBrowser",
        "com.intellij.ui.jcef.JBCefJSQuery",
    )

    /** True when every JCEF class the browser panel is built from resolves in this plugin's class loader. */
    fun isAvailable(): Boolean = requiredClasses.all(::loadable)

    private fun loadable(name: String): Boolean = try {
        Class.forName(name, false, JcefSupport::class.java.classLoader)
        true
    } catch (_: ClassNotFoundException) {
        false
    } catch (_: LinkageError) {
        false
    }
}
