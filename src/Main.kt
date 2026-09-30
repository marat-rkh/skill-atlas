import kotlin.system.exitProcess

fun main(args: Array<String>) {
    val repo = args.singleOrNull()?.let(GitHubRepo::parse)
    if (repo == null) {
        System.err.println("Usage: skill-atlas <github-repository-url>")
        exitProcess(2)
    }

    System.err.println("Analyzing ${repo.webUrl} ...")
    try {
        checkoutSkillFiles(repo).use { checkout ->
            print(renderMap(repo, checkout, loadSkillGroups(checkout)))
        }
    } catch (e: GitException) {
        System.err.println("error: ${e.message}")
        exitProcess(1)
    }
}

fun renderMap(repo: GitHubRepo, checkout: RepoCheckout, groups: List<SkillGroup>): String = buildString {
    val count = groups.sumOf { it.skills.size }
    appendLine("${repo.slug} · ${checkout.branch ?: "HEAD"} @ ${checkout.commit.take(7)} · $count skill${if (count == 1) "" else "s"}")
    if (groups.isEmpty()) appendLine("\nNo SKILL.md files found.")

    for (group in groups) {
        appendLine()
        appendLine(if (group.directory.isEmpty()) "./" else "${group.directory}/")
        group.skills.forEachIndexed { index, skill ->
            val last = index == group.skills.lastIndex
            appendLine((if (last) "└── " else "├── ") + skill.name)
            val indent = if (last) "    " else "│   "
            wrap(skill.description.ifBlank { "(no description)" }, width = 96).forEach { appendLine(indent + it) }
        }
    }
}

private fun wrap(text: String, width: Int): List<String> {
    val lines = mutableListOf<String>()
    val line = StringBuilder()
    for (word in text.split(Regex("\\s+")).filter { it.isNotEmpty() }) {
        if (line.isNotEmpty() && line.length + 1 + word.length > width) {
            lines += line.toString()
            line.clear()
        }
        if (line.isNotEmpty()) line.append(' ')
        line.append(word)
    }
    if (line.isNotEmpty()) lines += line.toString()
    return lines
}
