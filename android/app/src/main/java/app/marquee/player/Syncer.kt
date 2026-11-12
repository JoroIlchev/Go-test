package app.marquee.player

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.io.File

/** Keeps the manifest and the files fresh: syncs with the hub in a loop and
 *  downloads what the manifest needs. Everything survives a restart without
 *  network: the last manifest and the files stay on disk. */
class Syncer(
    context: Context,
    private val hub: Hub,
    private val listener: Listener,
) {
    interface Listener {
        fun onManifest(m: Manifest)
        fun onConnection(state: Connection)
        fun onFiles(ready: Int, total: Int)
    }

    enum class Connection { OK, OFFLINE, BLOCKED }

    private val info = DeviceInfo(context)
    private val manifestFile = File(context.filesDir, "manifest.json")
    val media = File(context.filesDir, "media").apply { mkdirs() }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val main = Handler(Looper.getMainLooper())
    private var downloads: Job? = null

    fun file(name: String) = File(media, name)

    /** The manifest from the last run, to play at once even without network. */
    fun cached(): Manifest? = try {
        if (manifestFile.exists()) Manifest.parse(manifestFile.readText()) else null
    } catch (e: Exception) {
        null
    }

    fun start() {
        val first = cached()
        first?.let { m -> fetch(m) }
        scope.launch {
            var version = first?.version ?: ""
            var backoff = 5_000L
            while (isActive) {
                try {
                    // The first call answers at once, so a fresh start reports itself quickly.
                    val (m, raw) = hub.sync(info.toJson(), version, wait = version.isNotEmpty())
                    post { listener.onConnection(Connection.OK) }
                    backoff = 5_000L
                    if (m.version != version) {
                        version = m.version
                        manifestFile.writeText(raw)
                        post { listener.onManifest(m) }
                        fetch(m)
                    }
                } catch (e: BlockedException) {
                    post { listener.onConnection(Connection.BLOCKED) }
                    delay(60_000L)
                } catch (e: Exception) {
                    Log.w(App.TAG, "sync failed: $e")
                    post { listener.onConnection(Connection.OFFLINE) }
                    delay(backoff)
                    backoff = (backoff * 2).coerceAtMost(60_000L)
                }
            }
        }
    }

    fun stop() = scope.cancel()

    /** Downloads the manifest's files in play order, retrying until all are
     *  here, then drops the files no longer needed. */
    private fun fetch(m: Manifest) {
        downloads?.cancel()
        downloads = scope.launch {
            while (isActive) {
                var failed = false
                for (f in m.files) {
                    val dest = file(f.name)
                    if (!dest.exists()) {
                        try {
                            hub.download(f, dest)
                        } catch (e: Exception) {
                            Log.w(App.TAG, "download failed: $e")
                            failed = true
                        }
                    }
                    val ready = m.files.count { file(it.name).exists() }
                    post { listener.onFiles(ready, m.files.size) }
                    if (!isActive) return@launch
                }
                if (!failed) break
                delay(30_000L)
            }
            val keep = m.files.map { it.name }.toSet()
            media.listFiles()?.filter { it.name !in keep }?.forEach { it.delete() }
        }
    }

    private fun post(block: () -> Unit) {
        main.post(block)
    }
}
