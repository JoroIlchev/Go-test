package app.marquee.player

import android.content.Context
import android.media.MediaCodecList
import android.os.Build
import android.os.StatFs
import android.os.SystemClock
import android.util.DisplayMetrics
import android.view.WindowManager
import org.json.JSONObject
import java.net.Inet4Address
import java.net.NetworkInterface

/** What the hub learns about the device, so it can warn about campaigns
 *  that will not look right on it. */
class DeviceInfo(private val context: Context) {
    // The hardware does not change while we run.
    private val video: Pair<Int, Int> by lazy { maxVideo("video/avc") ?: (0 to 0) }
    private val hevc: Boolean by lazy { maxVideo("video/hevc") != null }

    fun toJson(): JSONObject {
        val dm = DisplayMetrics()
        @Suppress("DEPRECATION")
        (context.getSystemService(Context.WINDOW_SERVICE) as WindowManager).defaultDisplay.getRealMetrics(dm)
        val fs = StatFs(context.filesDir.path)
        return JSONObject()
            .put("manufacturer", Build.MANUFACTURER)
            .put("model", Build.MODEL)
            .put("android", Build.VERSION.RELEASE)
            .put("sdk", Build.VERSION.SDK_INT)
            .put("app", BuildConfig.VERSION_NAME)
            .put("screen_w", dm.widthPixels)
            .put("screen_h", dm.heightPixels)
            .put("density", dm.densityDpi)
            .put("video_max_w", video.first)
            .put("video_max_h", video.second)
            .put("hevc", hevc)
            .put("free_bytes", fs.availableBytes)
            .put("total_bytes", fs.totalBytes)
            .put("local_ip", localIp())
            .put("uptime_s", SystemClock.elapsedRealtime() / 1000)
    }

    /** The largest common size a decoder plays at 30 fps, preferring hardware
     *  decoders (software ones are listed but stutter on cheap devices). */
    private fun maxVideo(mime: String): Pair<Int, Int>? {
        val sizes = listOf(3840 to 2160, 2560 to 1440, 1920 to 1080, 1280 to 720)
        val decoders = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos
            .filter { !it.isEncoder && it.supportedTypes.any { t -> t.equals(mime, ignoreCase = true) } }
        val hardware = decoders.filterNot { it.name.startsWith("OMX.google.") || it.name.startsWith("c2.android.") }
        for (list in listOf(hardware, decoders)) {
            for ((w, h) in sizes) {
                if (list.any { runCatching { it.getCapabilitiesForType(mime).videoCapabilities.areSizeAndRateSupported(w, h, 30.0) }.getOrDefault(false) }) {
                    return w to h
                }
            }
        }
        return null
    }

    private fun localIp(): String = try {
        NetworkInterface.getNetworkInterfaces().toList()
            .filter { it.isUp && !it.isLoopback }
            .flatMap { it.inetAddresses.toList() }
            .firstOrNull { it is Inet4Address }?.hostAddress ?: ""
    } catch (e: Exception) {
        ""
    }
}
