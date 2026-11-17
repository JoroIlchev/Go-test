package app.marquee.player

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import android.util.Log
import java.io.File
import java.security.MessageDigest

/** Installs the player the hub offers when it differs from this one. As the
 *  device owner it installs silently; otherwise Android asks on screen. */
class Updater(private val context: Context, private val hub: Hub) {
    private val own: String by lazy { sha256(File(context.applicationInfo.sourceDir)) }

    @Volatile
    private var busy = false

    /** Runs on a background thread, after every new manifest. */
    fun offer(app: MediaFile) {
        if (busy || app.sha256.isEmpty() || app.sha256 == own || skipped(context, app.sha256)) return
        busy = true
        try {
            val apk = File(context.cacheDir, "update.apk")
            hub.download(app, apk)
            // Asked again in a while if nobody answers the prompt.
            skip(context, app.sha256, System.currentTimeMillis() + RETRY_MS)
            install(apk, app.sha256)
            Log.i(App.TAG, "update ${app.sha256.take(12)} handed to the installer")
        } catch (e: Exception) {
            Log.w(App.TAG, "update failed: $e")
            skip(context, app.sha256, System.currentTimeMillis() + RETRY_MS)
        } finally {
            busy = false
        }
    }

    private fun install(apk: File, sha: String) {
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(context.packageName)
            if (Build.VERSION.SDK_INT >= 31) setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
        }
        val id = installer.createSession(params)
        installer.openSession(id).use { session ->
            apk.inputStream().use { input ->
                session.openWrite("player.apk", 0, apk.length()).use { out ->
                    input.copyTo(out)
                    session.fsync(out)
                }
            }
            val flags = PendingIntent.FLAG_UPDATE_CURRENT or (if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0)
            val result = Intent(context, UpdateReceiver::class.java).putExtra(EXTRA_SHA, sha)
            session.commit(PendingIntent.getBroadcast(context, id, result, flags).intentSender)
        }
        apk.delete()
    }

    companion object {
        const val EXTRA_SHA = "sha"
        private const val RETRY_MS = 60 * 60 * 1000L
        private const val KEY_SHA = "update_skip_sha"
        private const val KEY_UNTIL = "update_skip_until"

        private fun prefs(c: Context) = c.getSharedPreferences("marquee", Context.MODE_PRIVATE)

        /** Leaves this version alone until [until] (Long.MAX_VALUE: for good). */
        fun skip(c: Context, sha: String, until: Long) {
            prefs(c).edit().putString(KEY_SHA, sha).putLong(KEY_UNTIL, until).apply()
        }

        private fun skipped(c: Context, sha: String): Boolean {
            val p = prefs(c)
            return p.getString(KEY_SHA, null) == sha && System.currentTimeMillis() < p.getLong(KEY_UNTIL, 0)
        }

        private fun sha256(f: File): String {
            val digest = MessageDigest.getInstance("SHA-256")
            f.inputStream().use { input ->
                val buf = ByteArray(64 * 1024)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    digest.update(buf, 0, n)
                }
            }
            return digest.digest().joinToString("") { "%02x".format(it) }
        }
    }
}

/** The installer's answer: shows Android's prompt when it asks for one. */
class UpdateReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
        val sha = intent.getStringExtra(Updater.EXTRA_SHA) ?: ""
        when (status) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirm = intent.getParcelableExtra<Intent>(Intent.EXTRA_INTENT) ?: return
                context.startActivity(confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
            }
            // The new version starts itself (BootReceiver, MY_PACKAGE_REPLACED).
            PackageInstaller.STATUS_SUCCESS -> Log.i(App.TAG, "update installed")
            // Declined or not answered: asked again later.
            PackageInstaller.STATUS_FAILURE_ABORTED -> Log.i(App.TAG, "update declined")
            else -> {
                // Another signature, an older version...: this file will not install.
                Log.w(App.TAG, "update rejected: $status ${intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)}")
                Updater.skip(context, sha, Long.MAX_VALUE)
            }
        }
    }
}
