package app.marquee.player

import android.app.Activity
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings as AndroidSettings

/** What a player on a newer Android (TV sets) needs and cannot grant itself:
 *  someone allows it once, on the screen. */
object Perms {
    /** Android 10+ lets an app come to the front by itself (boot, screen on,
     *  after an update or a crash) only with "display over other apps". */
    fun canStartItself(c: Context) = Build.VERSION.SDK_INT < 29 || AndroidSettings.canDrawOverlays(c)

    /** Android 8+ installs updates only with "install unknown apps" for us. */
    fun canInstall(c: Context) = Build.VERSION.SDK_INT < 26 || c.packageManager.canRequestPackageInstalls()

    fun isTv(c: Context) = c.packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)

    fun allowStart(a: Activity) = open(a, AndroidSettings.ACTION_MANAGE_OVERLAY_PERMISSION)

    fun allowInstall(a: Activity) = open(a, AndroidSettings.ACTION_MANAGE_UNKNOWN_APP_SOURCES)

    /** Opens the permission screen for this app; some TV sets lack it, then
     *  the general list or the app's own page. */
    private fun open(a: Activity, action: String) {
        val app = Uri.parse("package:${a.packageName}")
        val tries = listOf(
            Intent(action, app),
            Intent(action),
            Intent(AndroidSettings.ACTION_APPLICATION_DETAILS_SETTINGS, app),
        )
        for (i in tries) {
            try {
                a.startActivity(i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                return
            } catch (e: ActivityNotFoundException) {
                // the next one
            }
        }
    }
}
