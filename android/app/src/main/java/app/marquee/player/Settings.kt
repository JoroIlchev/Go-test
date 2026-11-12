package app.marquee.player

import android.content.Context
import java.util.UUID

/** What the player keeps between starts: its id and the hub's address. */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("marquee", Context.MODE_PRIVATE)

    /** Random, made on first start; the hub knows the device by it, so it is
     *  the device's secret and never shown in full. */
    val deviceId: String
        get() = prefs.getString(KEY_ID, null) ?: UUID.randomUUID().toString().also {
            prefs.edit().putString(KEY_ID, it).apply()
        }

    var hubUrl: String
        get() = prefs.getString(KEY_HUB, "") ?: ""
        set(value) = prefs.edit().putString(KEY_HUB, normalizeHub(value)).apply()

    companion object {
        private const val KEY_ID = "device_id"
        private const val KEY_HUB = "hub_url"

        /** "192.168.1.5:8080" → "http://192.168.1.5:8080"; no trailing slash. */
        fun normalizeHub(raw: String): String {
            var s = raw.trim().trimEnd('/')
            if (s.isEmpty()) return ""
            if (!s.startsWith("http://") && !s.startsWith("https://")) s = "http://$s"
            // Someone may paste a link meant for another player; the hub address is its start.
            s = s.removeSuffix("/p/index.smil")
            return s.trimEnd('/')
        }
    }
}
