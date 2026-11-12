package app.marquee.player

import android.app.Activity
import android.app.AlertDialog
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.InputType
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView

/** The whole player: the stage plays content, the status screen covers it
 *  whenever there is nothing to play. Holding a click (or a finger) for
 *  3 seconds anywhere opens the settings, as does the Menu key. */
class MainActivity : Activity(), Syncer.Listener {
    private lateinit var settings: Settings
    private lateinit var status: StatusView
    private lateinit var root: FrameLayout
    private var stage: Stage? = null
    private var syncer: Syncer? = null
    private var hub: Hub? = null

    private var manifest: Manifest? = null
    private var connection = Syncer.Connection.OK
    private var files = 0 to 0
    private var playing = false

    private val main = Handler(Looper.getMainLooper())
    private val openSettings = Runnable { showSettings() }
    private var dialog: AlertDialog? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        settings = Settings(this)
        status = StatusView(this)
        root = FrameLayout(this)
        setContentView(root)
        root.addView(status, FrameLayout.LayoutParams(-1, -1))
        start()
    }

    private fun start() {
        stop()
        val url = settings.hubUrl
        status.footer("Marquee Player ${BuildConfig.VERSION_NAME} · ${url.ifEmpty { "без хъб" }}")
        if (url.isEmpty()) {
            status.show("Добре дошли", "Настройка", "Въведете адреса на хъба", 0.09f)
            main.post { showSettings() }
            return
        }
        val h = Hub(url, settings.deviceId)
        hub = h
        val s = Syncer(this, h, this)
        syncer = s
        val st = Stage(this, h::now, s::file) { on -> playing = on; render() }
        stage = st
        root.addView(st.view, 0, FrameLayout.LayoutParams(-1, -1))
        manifest = s.cached()
        manifest?.let { st.show(it) }
        render()
        s.start()
    }

    private fun stop() {
        syncer?.stop()
        syncer = null
        stage?.let {
            it.release()
            root.removeView(it.view)
        }
        stage = null
    }

    override fun onDestroy() {
        stop()
        super.onDestroy()
    }

    override fun onManifest(m: Manifest) {
        manifest = m
        stage?.show(m)
        render()
    }

    override fun onConnection(state: Syncer.Connection) {
        connection = state
        render()
    }

    override fun onFiles(ready: Int, total: Int) {
        files = ready to total
        stage?.fileReady()
        render()
    }

    /** What the status screen says, or hides it while content plays. */
    private fun render() {
        val m = manifest
        status.visibility = if (playing && connection != Syncer.Connection.BLOCKED) View.GONE else View.VISIBLE
        when {
            connection == Syncer.Connection.BLOCKED ->
                status.show("", "Блокирано", "Устройството е блокирано в хъба", 0.09f)
            m == null && connection == Syncer.Connection.OFFLINE ->
                status.show("Няма връзка с хъба", settings.hubUrl.removePrefix("http://"), "Опитва отново… · задръжте 3 сек. за настройки", 0.07f)
            m == null -> status.show("", "Свързване…", "", 0.09f)
            m.status == "pending" ->
                status.show("Код за свързване", m.pairingCode, "Въведете кода в marquee → Чакащи устройства")
            files.second > 0 && files.first < files.second && m.segmentAt(hub?.now() ?: 0L) != null ->
                status.show("Сваляне на съдържанието", "${files.first} от ${files.second}", m.name, 0.12f)
            else ->
                status.show("Устройството е свързано", m.name.ifEmpty { "Без име" },
                    listOf(m.site, "Очаква съдържание").filter { it.isNotEmpty() }.joinToString(" · "), 0.08f)
        }
    }

    override fun dispatchTouchEvent(ev: MotionEvent): Boolean {
        when (ev.actionMasked) {
            MotionEvent.ACTION_DOWN -> main.postDelayed(openSettings, 3000)
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> main.removeCallbacks(openSettings)
        }
        return super.dispatchTouchEvent(ev)
    }

    override fun onKeyUp(keyCode: Int, event: KeyEvent): Boolean {
        if (keyCode == KeyEvent.KEYCODE_MENU || keyCode == KeyEvent.KEYCODE_S) {
            showSettings()
            return true
        }
        return super.onKeyUp(keyCode, event)
    }

    // A player has nowhere to go back to.
    @Deprecated("Activity API")
    override fun onBackPressed() {
        dialog?.dismiss()
    }

    private fun showSettings() {
        if (dialog?.isShowing == true) return
        val pad = (resources.displayMetrics.density * 20).toInt()
        val input = EditText(this).apply {
            setText(settings.hubUrl)
            inputType = InputType.TYPE_CLASS_TEXT or InputType.TYPE_TEXT_VARIATION_URI
            hint = "http://192.168.1.10:8080"
            setSingleLine()
        }
        val about = TextView(this).apply {
            text = "Устройство ${settings.deviceId.take(8)} · версия ${BuildConfig.VERSION_NAME}"
            alpha = 0.7f
            setPadding(0, pad / 2, 0, 0)
        }
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad / 2, pad, 0)
            addView(TextView(this@MainActivity).apply { text = "Адрес на хъба, например 192.168.1.10:8080" })
            addView(input)
            addView(about)
        }
        dialog = AlertDialog.Builder(this, android.R.style.Theme_DeviceDefault_Dialog_Alert)
            .setTitle("Настройки на Marquee Player")
            .setView(box)
            .setPositiveButton("Запази") { _, _ ->
                val changed = Settings.normalizeHub(input.text.toString()) != settings.hubUrl
                settings.hubUrl = input.text.toString()
                if (changed) {
                    manifest = null
                    connection = Syncer.Connection.OK
                    start()
                }
            }
            .setNegativeButton("Отказ", null)
            .show()
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus) immersive()
    }

    @Suppress("DEPRECATION")
    private fun immersive() {
        window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or
            View.SYSTEM_UI_FLAG_FULLSCREEN or View.SYSTEM_UI_FLAG_HIDE_NAVIGATION or
            View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN or View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION or
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
    }
}
