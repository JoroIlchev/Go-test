package app.marquee.player

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Color
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.widget.FrameLayout
import android.widget.ImageView
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import java.io.File
import java.util.concurrent.Executors

/** Plays the current segment of the manifest: pictures cross-fade, videos
 *  start on their first frame, so nothing flashes black between items. */
@androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
class Stage(
    context: Context,
    private val clock: () -> Long,
    private val fileOf: (String) -> File,
    private val onPlaying: (Boolean) -> Unit,
) {
    val view = FrameLayout(context).apply { setBackgroundColor(Color.BLACK) }

    private val main = Handler(Looper.getMainLooper())
    private val decoder = Executors.newSingleThreadExecutor()

    private val video = ExoPlayer.Builder(context).build()
    private val music = ExoPlayer.Builder(context).build().apply { repeatMode = Player.REPEAT_MODE_ONE }
    private val videoView = PlayerView(context).apply {
        useController = false
        player = video
        setShutterBackgroundColor(Color.BLACK)
        setKeepContentOnPlayerReset(true)
        // Always visible under the pictures: a hidden view gets no surface,
        // and without one the first frame never comes.
    }
    // Two picture layers: the next one fades in over the current one.
    // Black behind letterboxed pictures, so a stopped video never shows through.
    private val pictures = List(2) { ImageView(context).apply { alpha = 0f; setBackgroundColor(Color.BLACK) } }
    private var front = 0

    private var manifest: Manifest? = null
    private var segment: Segment? = null
    private var index = -1
    private var current: Item? = null
    private var playing = false
    private var generation = 0 // bumps on every item, so stale timers do nothing

    private val screenW = context.resources.displayMetrics.widthPixels
    private val screenH = context.resources.displayMetrics.heightPixels

    init {
        view.addView(videoView, FrameLayout.LayoutParams(-1, -1))
        pictures.forEach { view.addView(it, FrameLayout.LayoutParams(-1, -1)) }
        video.addListener(object : Player.Listener {
            override fun onRenderedFirstFrame() {
                // The video is on screen: let the picture above it go.
                pictures.forEach { p -> p.animate().alpha(0f).setDuration(FADE_MS).withEndAction { p.setImageDrawable(null) } }
            }

            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_ENDED && current?.durationMs == 0L) next()
            }

            override fun onPlayerError(error: PlaybackException) {
                Log.w(App.TAG, "video failed: ${current?.file}: $error")
                val g = generation
                main.postDelayed({ if (g == generation) next() }, 1000)
            }
        })
        tick()
    }

    fun show(m: Manifest) {
        manifest = m
        check()
    }

    /** A file arrived: start playing if we were waiting for one. */
    fun fileReady() {
        if (!playing) next()
    }

    fun release() {
        main.removeCallbacksAndMessages(null)
        decoder.shutdownNow()
        video.release()
        music.release()
    }

    private fun tick() {
        check()
        main.postDelayed(::tick, 1000)
    }

    /** Switches segments when the schedule says so (on the hub's clock). */
    private fun check() {
        val seg = manifest?.takeIf { it.status == "active" }?.segmentAt(clock())
        if (seg == segment) return
        segment = seg
        index = -1
        startMusic(seg)
        next()
    }

    private fun startMusic(seg: Segment?) {
        val f = seg?.audio?.let(fileOf)
        if (f == null || !f.exists()) {
            music.stop()
            video.volume = 1f
            return
        }
        music.setMediaItem(MediaItem.fromUri(Uri.fromFile(f)))
        music.prepare()
        music.play()
        video.volume = 0f // the background music plays instead of the videos' sound
    }

    private fun next() {
        generation++
        val items = segment?.items.orEmpty()
        val ready = items.indices.filter { fileOf(items[it].file).exists() }
        if (ready.isEmpty()) {
            stopAll()
            return
        }
        index = ready.firstOrNull { it > index } ?: ready.first()
        val it = items[index]
        // One video alone loops without a gap; one picture just stays.
        if (ready.size == 1 && playing && current == it) {
            if (it.durationMs > 0 || isPicture(it)) schedule(it)
            return
        }
        current = it
        setPlaying(true)
        if (isPicture(it)) playPicture(it) else playVideo(it, loop = ready.size == 1)
    }

    private fun isPicture(it: Item) = manifest?.files?.firstOrNull { f -> f.name == it.file }?.kind != "video"

    private fun playPicture(it: Item) {
        val g = generation
        val f = fileOf(it.file)
        decoder.execute {
            val bmp = decode(f)
            main.post {
                if (g != generation) return@post
                if (bmp == null) {
                    Log.w(App.TAG, "picture failed: ${it.file}")
                    main.postDelayed({ if (g == generation) next() }, 1000)
                    return@post
                }
                val back = pictures[front]
                front = 1 - front
                val pic = pictures[front]
                pic.scaleType = when (it.fit) {
                    "slice" -> ImageView.ScaleType.CENTER_CROP
                    "fill" -> ImageView.ScaleType.FIT_XY
                    else -> ImageView.ScaleType.FIT_CENTER
                }
                pic.setImageBitmap(bmp)
                pic.alpha = 0f
                pic.bringToFront()
                pic.animate().alpha(1f).setDuration(FADE_MS).withEndAction {
                    if (g != generation) return@withEndAction
                    back.alpha = 0f
                    back.setImageDrawable(null)
                    video.stop()
                }
                schedule(it)
            }
        }
    }

    private fun playVideo(it: Item, loop: Boolean) {
        videoView.resizeMode = when (it.fit) {
            "slice" -> AspectRatioFrameLayout.RESIZE_MODE_ZOOM
            "fill" -> AspectRatioFrameLayout.RESIZE_MODE_FILL
            else -> AspectRatioFrameLayout.RESIZE_MODE_FIT
        }
        video.repeatMode = if (loop) Player.REPEAT_MODE_ONE else Player.REPEAT_MODE_OFF
        video.setMediaItem(MediaItem.fromUri(Uri.fromFile(fileOf(it.file))))
        video.prepare()
        video.play()
        if (it.durationMs > 0 && !loop) schedule(it)
    }

    private fun schedule(it: Item) {
        val g = generation
        val ms = if (it.durationMs > 0) it.durationMs else 10_000L
        main.postDelayed({ if (g == generation) next() }, ms)
    }

    private fun stopAll() {
        current = null
        video.stop()
        pictures.forEach { it.animate().cancel(); it.alpha = 0f; it.setImageDrawable(null) }
        setPlaying(false)
    }

    private fun setPlaying(on: Boolean) {
        if (on == playing) return
        playing = on
        onPlaying(on)
    }

    /** Decodes no larger than needed for the screen: cheap devices run out of
     *  memory on phone photos otherwise. */
    private fun decode(f: File): Bitmap? = try {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeFile(f.path, bounds)
        var sample = 1
        val maxSide = maxOf(screenW, screenH)
        while (maxOf(bounds.outWidth, bounds.outHeight) / (sample * 2) >= maxSide) sample *= 2
        BitmapFactory.decodeFile(f.path, BitmapFactory.Options().apply { inSampleSize = sample })
    } catch (e: OutOfMemoryError) {
        null
    }

    companion object {
        private const val FADE_MS = 600L
    }
}
