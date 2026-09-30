/** An agent skill: a directory with a SKILL.md file. [path] is the skill directory ("" for the repository root). */
data class Skill(val name: String, val description: String, val path: String)

/** Skills that live side by side in the same [directory], e.g. `.claude/skills`. */
data class SkillGroup(val directory: String, val skills: List<Skill>)

fun loadSkillGroups(checkout: RepoCheckout): List<SkillGroup> =
    checkout.skillFiles
        .map { file -> parseSkill(file.substringBeforeLast('/', ""), checkout.read(file)) }
        .groupBy { it.path.substringBeforeLast('/', "") }
        .toSortedMap()
        .map { (directory, skills) -> SkillGroup(directory, skills.sortedBy { it.name }) }

fun parseSkill(path: String, content: String): Skill {
    val frontmatter = parseFrontmatter(content)
    return Skill(
        name = frontmatter["name"]?.takeIf { it.isNotBlank() } ?: path.substringAfterLast('/').ifEmpty { "(unnamed)" },
        description = frontmatter["description"].orEmpty(),
        path = path,
    )
}

private val keyLine = Regex("""([\w-]+):(.*)""")

/** Reads the top-level scalar keys of a YAML frontmatter block. Nested structures are not interpreted. */
fun parseFrontmatter(content: String): Map<String, String> {
    val lines = content.removePrefix("﻿").lines()
    if (lines.firstOrNull()?.trimEnd() != "---") return emptyMap()
    val end = lines.drop(1).indexOfFirst { it.trimEnd() == "---" }
    if (end < 0) return emptyMap()
    val block = lines.subList(1, end + 1)

    val result = linkedMapOf<String, String>()
    var i = 0
    while (i < block.size) {
        val match = keyLine.matchEntire(block[i++]) ?: continue
        // Indented (or blank) lines that follow a key belong to its value.
        val continuation = mutableListOf<String>()
        while (i < block.size && (block[i].isBlank() || block[i].first().isWhitespace())) continuation += block[i++]
        result[match.groupValues[1]] = scalarValue(match.groupValues[2].trim(), continuation.dropLastWhile { it.isBlank() })
    }
    return result
}

private fun scalarValue(value: String, continuation: List<String>): String {
    if (value.startsWith("|") || value.startsWith(">")) {
        val separator = if (value.startsWith("|")) "\n" else " "
        return continuation.joinToString(separator) { it.trim() }.trim()
    }
    val text = (listOf(value) + continuation.map { it.trim() }).filter { it.isNotEmpty() }.joinToString(" ")
    return when {
        text.length >= 2 && text.startsWith('"') && text.endsWith('"') ->
            text.substring(1, text.length - 1).replace(Regex("""\\(.)""")) {
                when (val c = it.groupValues[1]) { "n" -> "\n"; "t" -> "\t"; else -> c }
            }
        text.length >= 2 && text.startsWith('\'') && text.endsWith('\'') ->
            text.substring(1, text.length - 1).replace("''", "'")
        else -> text
    }
}
