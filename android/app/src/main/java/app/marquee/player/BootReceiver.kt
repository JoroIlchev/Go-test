package app.marquee.player

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/** Starts the player when the device boots, even if it is not the home screen. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        context.startActivity(Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}
