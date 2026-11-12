package app.marquee.player

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.RectF
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView

/** The branded screen shown when there is nothing to play: the pairing
 *  code, "waiting for content", "no connection". Matches the hub's pair page. */
class StatusView(context: Context) : FrameLayout(context) {
    private val hint = text(0.045f, 0xB3EEEEEE.toInt())
    private val big = text(0.18f, 0xFFC4B5FD.toInt(), bold = true)
    private val sub = text(0.04f, 0xB3EEEEEE.toInt())
    private val footer = text(0.022f, 0x66EEEEEE.toInt())

    init {
        val h = resources.displayMetrics.heightPixels
        background = GradientDrawable().apply {
            gradientType = GradientDrawable.RADIAL_GRADIENT
            colors = intArrayOf(0xFF2E1A52.toInt(), 0xFF120C1F.toInt())
            gradientRadius = h * 0.9f
            setGradientCenter(0.5f, 0.45f)
        }
        val logo = LinearLayout(context).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            addView(MarkView(context), LinearLayout.LayoutParams((h * 0.06f).toInt(), (h * 0.06f).toInt()))
            addView(text(0.042f, Color.WHITE, bold = true).apply {
                text = "marquee"
                setPadding((h * 0.016f).toInt(), 0, 0, 0)
            })
        }
        addView(logo, LayoutParams(-2, -2, Gravity.TOP or Gravity.START).apply {
            val m = (h * 0.05f).toInt()
            setMargins(m, m, m, m)
        })
        val column = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER
            addView(hint)
            addView(big)
            addView(sub)
        }
        addView(column, LayoutParams(-1, -2, Gravity.CENTER))
        addView(footer, LayoutParams(-2, -2, Gravity.BOTTOM or Gravity.END).apply {
            val m = (h * 0.03f).toInt()
            setMargins(m, m, m, m)
        })
    }

    /** [bigSize] is a share of the screen height: the code is huge, a name smaller. */
    fun show(hint: String, big: String, sub: String, bigSize: Float = 0.18f) {
        this.hint.text = hint
        this.big.text = big
        this.big.setTextSize(TypedValue.COMPLEX_UNIT_PX, resources.displayMetrics.heightPixels * bigSize)
        this.big.letterSpacing = if (bigSize >= 0.15f) 0.1f else 0f
        this.sub.text = sub
        this.hint.visibility = if (hint.isEmpty()) View.GONE else View.VISIBLE
        this.sub.visibility = if (sub.isEmpty()) View.GONE else View.VISIBLE
    }

    fun footer(s: String) {
        footer.text = s
    }

    private fun text(size: Float, color: Int, bold: Boolean = false) = TextView(context).apply {
        setTextColor(color)
        setTextSize(TypedValue.COMPLEX_UNIT_PX, resources.displayMetrics.heightPixels * size)
        gravity = Gravity.CENTER
        if (bold) typeface = Typeface.DEFAULT_BOLD
        includeFontPadding = false
        setPadding(0, (resources.displayMetrics.heightPixels * 0.012f).toInt(), 0, 0)
    }
}

/** The marquee mark: rings of light on the brand purple (as the favicon). */
class MarkView(context: Context) : View(context) {
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val rect = RectF()

    override fun onDraw(canvas: Canvas) {
        val u = width / 64f
        paint.style = Paint.Style.FILL
        paint.color = 0xFF7C3AED.toInt()
        rect.set(0f, 0f, width.toFloat(), height.toFloat())
        canvas.drawRoundRect(rect, 16 * u, 16 * u, paint)
        val c = 32 * u
        paint.style = Paint.Style.STROKE
        paint.color = Color.argb(71, 255, 255, 255)
        paint.strokeWidth = 3 * u
        canvas.drawCircle(c, c, 22 * u, paint)
        paint.color = Color.argb(153, 255, 255, 255)
        paint.strokeWidth = 3.5f * u
        canvas.drawCircle(c, c, 14 * u, paint)
        paint.style = Paint.Style.FILL
        paint.color = Color.WHITE
        canvas.drawCircle(c, c, 7 * u, paint)
    }
}
