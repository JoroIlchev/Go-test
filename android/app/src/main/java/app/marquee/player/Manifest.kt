package app.marquee.player

import org.json.JSONArray
import org.json.JSONObject
import java.text.SimpleDateFormat
import java.util.Locale

/** What the hub tells the player to show; see the hub's internal/player/api.go. */
data class Manifest(
    val version: String,
    val status: String, // pending | active
    val pairingCode: String,
    val name: String,
    val site: String,
    val segments: List<Segment>,
    val files: List<MediaFile>,
) {
    /** The segment playing at [now] (hub time), if any. */
    fun segmentAt(now: Long): Segment? = segments.lastOrNull { it.start <= now && (it.end == null || now < it.end) }

    companion object {
        fun parse(json: String): Manifest {
            val o = JSONObject(json)
            return Manifest(
                version = o.optString("version"),
                status = o.optString("status"),
                pairingCode = o.optString("pairing_code"),
                name = o.optString("name"),
                site = o.optString("site"),
                segments = o.optJSONArray("segments").objects().map { s ->
                    Segment(
                        start = parseTime(s.optString("start")) ?: 0L,
                        end = parseTime(s.optString("end")),
                        items = s.optJSONArray("items").objects().map {
                            Item(it.optString("file"), it.optLong("duration_ms"), it.optString("fit", "meet"))
                        },
                        audio = s.optString("audio").ifEmpty { null },
                    )
                },
                files = o.optJSONArray("files").objects().map {
                    MediaFile(
                        name = it.optString("name"),
                        url = it.optString("url"),
                        sha256 = it.optString("sha256"),
                        size = it.optLong("size"),
                        kind = it.optString("kind"),
                    )
                },
            )
        }

        private fun JSONArray?.objects(): List<JSONObject> =
            if (this == null) emptyList() else (0 until length()).map { getJSONObject(it) }

        /** RFC 3339 as the hub writes it ("2026-11-12T08:00:00+02:00" or "...Z"). */
        private fun parseTime(s: String): Long? {
            if (s.isEmpty()) return null
            return try {
                SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssXXX", Locale.US).parse(s)?.time
            } catch (e: Exception) {
                null
            }
        }
    }
}

data class Segment(val start: Long, val end: Long?, val items: List<Item>, val audio: String?)

/** One entry of the loop. durationMs 0: a video plays to its end. */
data class Item(val file: String, val durationMs: Long, val fit: String)

data class MediaFile(val name: String, val url: String, val sha256: String, val size: Long, val kind: String)
