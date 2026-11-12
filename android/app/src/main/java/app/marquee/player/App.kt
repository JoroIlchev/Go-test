package app.marquee.player

import android.app.AlarmManager
import android.app.Application
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.util.Log
import kotlin.system.exitProcess

/** A screen must never stay dark: after a crash the player starts again. */
class App : Application() {
    override fun onCreate() {
        super.onCreate()
        val previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, error ->
            Log.e(TAG, "crash, restarting in 3 s", error)
            try {
                restartSoon(this)
            } finally {
                if (previous != null && thread.name != "main") previous.uncaughtException(thread, error)
                exitProcess(2)
            }
        }
    }

    companion object {
        const val TAG = "marquee"

        fun restartSoon(context: Context) {
            val intent = Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            val pending = PendingIntent.getActivity(
                context, 0, intent,
                PendingIntent.FLAG_CANCEL_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            val alarms = context.getSystemService(Context.ALARM_SERVICE) as AlarmManager
            alarms.set(AlarmManager.RTC, System.currentTimeMillis() + 3000, pending)
        }
    }
}
