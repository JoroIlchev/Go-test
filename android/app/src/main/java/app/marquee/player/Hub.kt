package app.marquee.player

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.File
import java.io.IOException
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

class BlockedException : IOException("the device is blocked in the hub")

/** Talks to the hub: the sync call and file downloads. */
class Hub(private val base: String, private val deviceId: String) {
    private val client = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        // The hub holds a waiting sync for up to 50 s.
        .readTimeout(80, TimeUnit.SECONDS)
        .build()

    /** Hub clock minus ours, from the Date header: schedules follow the hub
     *  even when the device's clock is wrong. */
    @Volatile
    var clockOffset = 0L
        private set

    fun now() = System.currentTimeMillis() + clockOffset

    /** Reports the device and returns the manifest. With [wait] the hub answers
     *  when the manifest differs from [version], or after about 50 s. */
    fun sync(info: JSONObject, version: String, wait: Boolean): Pair<Manifest, String> {
        val body = JSONObject().put("info", info).put("version", version).put("wait", wait)
        val req = Request.Builder()
            .url("$base/api/v1/sync")
            .header("X-Device-Id", deviceId)
            .post(body.toString().toRequestBody("application/json".toMediaType()))
            .build()
        client.newCall(req).execute().use { resp ->
            if (resp.code == 403) throw BlockedException()
            if (!resp.isSuccessful) throw IOException("hub answered ${resp.code}")
            resp.headers.getDate("Date")?.let { clockOffset = it.time - System.currentTimeMillis() }
            val raw = resp.body?.string() ?: throw IOException("empty answer")
            return Manifest.parse(raw) to raw
        }
    }

    /** Downloads [f] into [dest], checking its SHA-256; nothing is left behind on failure. */
    fun download(f: MediaFile, dest: File) {
        val part = File(dest.path + ".part")
        val req = Request.Builder().url(f.url).build()
        client.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) throw IOException("${f.name}: hub answered ${resp.code}")
            val digest = MessageDigest.getInstance("SHA-256")
            resp.body!!.byteStream().use { input ->
                part.outputStream().use { out ->
                    val buf = ByteArray(64 * 1024)
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        digest.update(buf, 0, n)
                        out.write(buf, 0, n)
                    }
                }
            }
            val sum = digest.digest().joinToString("") { "%02x".format(it) }
            if (f.sha256.isNotEmpty() && sum != f.sha256) {
                part.delete()
                throw IOException("${f.name}: checksum mismatch")
            }
            if (!part.renameTo(dest)) {
                part.delete()
                throw IOException("${f.name}: could not store")
            }
        }
    }
}
