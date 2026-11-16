package dev.foxxycode.intellij.process

/** The HTTP/API dialect of the binary the panel launched. */
enum class BackendKind(val apiPrefix: String) {
    FOXXYCODE("foxxycode"),
    CODDY("coddy"),
    ;

    val hasNativeIdeApi: Boolean get() = this == FOXXYCODE

    companion object {
        /**
         * `-v` prints only a number in both projects, so the command's own help is the reliable
         * discriminator. Match the usage line rather than any mention of Coddy in prose.
         */
        fun detect(httpHelp: String): BackendKind {
            val coddyUsage = Regex("(?im)^\\s*(?:\\S*[\\\\/])?coddy(?:\\.exe)?\\s+(?:http|serve)\\b")
            return if (coddyUsage.containsMatchIn(httpHelp)) CODDY else FOXXYCODE
        }
    }
}
