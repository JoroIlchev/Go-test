package app.marquee.player

import android.app.admin.DeviceAdminReceiver
import android.app.admin.DevicePolicyManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

/** Device owner, set once from a computer:
 *  adb shell dpm set-device-owner app.marquee.player/.Admin
 *  Updates from the hub then install without anyone at the screen. */
class Admin : DeviceAdminReceiver()

/** Gives the device owner up, so the player can be uninstalled again:
 *  adb shell am broadcast -a app.marquee.player.RELEASE_OWNER -n app.marquee.player/.ReleaseOwner */
class ReleaseOwner : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val dpm = context.getSystemService(Context.DEVICE_POLICY_SERVICE) as DevicePolicyManager
        if (dpm.isDeviceOwnerApp(context.packageName)) {
            @Suppress("DEPRECATION")
            dpm.clearDeviceOwnerApp(context.packageName)
            Log.i(App.TAG, "no longer the device owner")
        }
    }
}
