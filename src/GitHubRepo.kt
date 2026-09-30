/** A GitHub repository identified by its owner and name. */
data class GitHubRepo(val owner: String, val name: String) {
    val slug get() = "$owner/$name"
    val webUrl get() = "https://github.com/$owner/$name"
    val cloneUrl get() = "$webUrl.git"

    companion object {
        // Accepts https://github.com/o/r, github.com/o/r, git@github.com:o/r.git; anything after o/r is ignored.
        private val urlPattern = Regex(
            """(?:(?:https?://)?(?:www\.)?github\.com/|git@github\.com:)([\w.-]+)/([\w.-]+?)(?:\.git)?(?:[/?#].*)?""",
            RegexOption.IGNORE_CASE,
        )

        fun parse(url: String): GitHubRepo? =
            urlPattern.matchEntire(url.trim())?.let { GitHubRepo(it.groupValues[1], it.groupValues[2]) }
    }
}
