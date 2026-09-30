import java.io.IOException
import java.nio.file.Files
import java.nio.file.Path
import kotlin.concurrent.thread

class GitException(message: String) : Exception(message)

/** A temporary shallow checkout of a repository that contains only its SKILL.md files. */
class RepoCheckout(
    private val dir: Path,
    val branch: String?,
    val commit: String,
    val skillFiles: List<String>,
) : AutoCloseable {
    fun read(path: String): String = dir.resolve(path).toFile().readText()

    override fun close() {
        dir.toFile().deleteRecursively()
    }
}

/**
 * Fetches the latest commit of the default branch without file contents, finds every SKILL.md in its tree,
 * and then downloads just those files. This keeps huge repositories cheap to analyze.
 */
fun checkoutSkillFiles(repo: GitHubRepo): RepoCheckout {
    val dir = Files.createTempDirectory("skill-atlas-")
    try {
        git(dir, "init", "-q")
        git(dir, "remote", "add", "origin", repo.cloneUrl)
        val head = try {
            git(dir, "ls-remote", "--symref", "origin", "HEAD")
        } catch (e: GitException) {
            throw GitException("cannot access ${repo.webUrl} (repository not found or private)\n${e.message}")
        }
        val branch = head.lineSequence()
            .firstOrNull { it.startsWith("ref: refs/heads/") }
            ?.removePrefix("ref: refs/heads/")
            ?.substringBefore('\t')

        git(dir, "fetch", "-q", "--depth", "1", "--filter=blob:none", "origin", "HEAD")
        val commit = git(dir, "rev-parse", "FETCH_HEAD").trim()

        // Entries look like "<mode> <type> <object>\t<path>"; mode 100xxx is a regular file.
        val skillFiles = git(dir, "ls-tree", "-r", "-z", "FETCH_HEAD").split('\u0000')
            .mapNotNull { entry ->
                val (meta, path) = entry.split('\t', limit = 2).takeIf { it.size == 2 } ?: return@mapNotNull null
                path.takeIf { meta.startsWith("100") && it.substringAfterLast('/') == "SKILL.md" }
            }

        if (skillFiles.isNotEmpty()) {
            // Checking out through sparse patterns downloads all the needed blobs in a single batch.
            git(dir, "sparse-checkout", "set", "--no-cone", "--stdin", input = skillFiles.joinToString("\n", transform = ::sparsePattern))
            git(dir, "checkout", "-q", "FETCH_HEAD")
        }
        return RepoCheckout(dir, branch, commit, skillFiles)
    } catch (e: Throwable) {
        dir.toFile().deleteRecursively()
        throw e
    }
}

private fun sparsePattern(path: String) = "/" + path.replace(Regex("""[\\*?\[]""")) { "\\" + it.value }

private fun git(dir: Path, vararg args: String, input: String? = null): String {
    val process = try {
        ProcessBuilder(listOf("git", "-C", dir.toString()) + args)
            .apply { environment()["GIT_TERMINAL_PROMPT"] = "0" }
            .start()
    } catch (e: IOException) {
        throw GitException("git is required but could not be started: ${e.message}")
    }
    var stderr = ""
    val stderrReader = thread { stderr = process.errorStream.bufferedReader().readText() }
    process.outputStream.use { out -> input?.let { out.write(it.toByteArray()) } }
    val stdout = process.inputStream.bufferedReader().readText()
    stderrReader.join()
    if (process.waitFor() != 0) throw GitException("git ${args.first()} failed: ${stderr.trim()}")
    return stdout
}
