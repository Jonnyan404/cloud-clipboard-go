package com.cloudclip

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Bitmap
import android.graphics.Color
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.*
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.localbroadcastmanager.content.LocalBroadcastManager
import com.google.android.material.bottomnavigation.BottomNavigationView
import com.google.android.material.floatingactionbutton.FloatingActionButton
import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.common.BitMatrix
import com.google.zxing.qrcode.QRCodeWriter
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class MainActivity : AppCompatActivity() {
    private lateinit var pageServices: View
    private lateinit var pageSync: View
    private lateinit var pageSettings: View
    private lateinit var bottomNav: BottomNavigationView
    private lateinit var powerFab: FloatingActionButton
    private lateinit var statusChip: TextView
    private lateinit var coverTitle: TextView
    private lateinit var coverSubtitle: TextView
    private lateinit var statDevices: TextView
    private lateinit var statTodaySyncs: TextView
    private lateinit var statLatency: TextView
    private lateinit var infoLan: TextView
    private lateinit var infoPort: TextView
    private lateinit var infoAuth: TextView
    private lateinit var infoDir: TextView
    private lateinit var qrImage: ImageView
    private lateinit var addressText: TextView
    private lateinit var addressActions: View
    private lateinit var portInput: EditText
    private lateinit var authInput: EditText
    private lateinit var storageDirText: TextView
    private lateinit var historyFileText: TextView
    private lateinit var copyAddressButton: TextView
    private lateinit var openBrowserButton: TextView
    private lateinit var githubButton: TextView
    private lateinit var helpButton: TextView
    private lateinit var syncEmpty: TextView
    private lateinit var syncList: ListView

    private val handler = Handler(Looper.getMainLooper())
    private val updateRunnable = object : Runnable {
        override fun run() {
            updateStatus()
            if (ClipboardService.isRunning) {
                handler.postDelayed(this, 1000)
            }
        }
    }

    private val serviceStoppedReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action == ClipboardService.ACTION_SERVICE_STOPPED) {
                updateStatus()
            }
        }
    }

    private val openDirectoryLauncher = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            result.data?.data?.also { uri ->
                contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
                val prefs = getSharedPreferences("config", MODE_PRIVATE).edit()
                prefs.putString("storageDirUri", uri.toString())
                prefs.apply()
                storageDirText.text = CloudClipboardPaths.shortDirName(CloudClipboardPaths.getPathFromUri(this, uri))
                historyFileText.text = CloudClipboardPaths.shortDirName(CloudClipboardPaths.getPathFromUri(this, uri)) + File.separator + "history.json"
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        pageServices = findViewById(R.id.pageServices)
        pageSync = findViewById(R.id.pageSync)
        pageSettings = findViewById(R.id.pageSettings)
        bottomNav = findViewById(R.id.bottomNav)
        powerFab = findViewById(R.id.powerFab)
        statusChip = findViewById(R.id.statusChip)
        coverTitle = findViewById(R.id.coverTitle)
        coverSubtitle = findViewById(R.id.coverSubtitle)
        statDevices = findViewById(R.id.statDevices)
        statTodaySyncs = findViewById(R.id.statTodaySyncs)
        statLatency = findViewById(R.id.statLatency)
        infoLan = findViewById(R.id.infoLan)
        infoPort = findViewById(R.id.infoPort)
        infoAuth = findViewById(R.id.infoAuth)
        infoDir = findViewById(R.id.infoDir)
        qrImage = findViewById(R.id.qrImage)
        addressText = findViewById(R.id.addressText)
        addressActions = findViewById(R.id.addressActions)
        portInput = findViewById(R.id.portInput)
        authInput = findViewById(R.id.authInput)
        storageDirText = findViewById(R.id.storageDirText)
        historyFileText = findViewById(R.id.historyFileText)
        copyAddressButton = findViewById(R.id.copyAddressButton)
        openBrowserButton = findViewById(R.id.openBrowserButton)
        githubButton = findViewById(R.id.githubButton)
        helpButton = findViewById(R.id.helpButton)
        syncEmpty = findViewById(R.id.syncEmpty)
        syncList = findViewById(R.id.syncList)

        loadConfig()

        // 底部导航切换
        bottomNav.setOnItemSelectedListener { item ->
            when (item.itemId) {
                R.id.nav_services -> {
                    showPage(pageServices)
                    true
                }
                R.id.nav_sync -> {
                    showPage(pageSync)
                    loadHistory()
                    true
                }
                R.id.nav_settings -> {
                    showPage(pageSettings)
                    true
                }
                else -> false
            }
        }

        // 悬浮电源按钮: 启动/停止
        powerFab.setOnClickListener { toggleService() }

        storageDirText.setOnClickListener {
            val intent = Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
            openDirectoryLauncher.launch(intent)
        }
        historyFileText.setOnClickListener { storageDirText.performClick() }

        githubButton.setOnClickListener {
            val intent = Intent(Intent.ACTION_VIEW, Uri.parse("https://github.com/Jonnyan404/cloud-clipboard-go"))
            startActivity(intent)
        }

        helpButton.setOnClickListener {
            val intent = Intent(Intent.ACTION_VIEW, Uri.parse("https://github.com/Jonnyan404/cloud-clipboard-go#readme"))
            startActivity(intent)
        }

        openBrowserButton.setOnClickListener {
            val url = addressText.text.toString()
            if (url.isNotEmpty()) {
                val intent = Intent(Intent.ACTION_VIEW, Uri.parse(url))
                startActivity(intent)
            }
        }

        copyAddressButton.setOnClickListener {
            val url = addressText.text.toString()
            if (url.isNotEmpty()) {
                val clipboard = getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                val clip = ClipData.newPlainText("Server Address", url)
                clipboard.setPrimaryClip(clip)
                Toast.makeText(this, R.string.address_copied, Toast.LENGTH_SHORT).show()
            }
        }
    }

    private fun showPage(page: View) {
        pageServices.visibility = if (page === pageServices) View.VISIBLE else View.GONE
        pageSync.visibility = if (page === pageSync) View.VISIBLE else View.GONE
        pageSettings.visibility = if (page === pageSettings) View.VISIBLE else View.GONE
        if (page === pageServices) {
            updateStatus()
        }
    }

    private fun toggleService() {
        if (ClipboardService.isRunning) {
            stopService(Intent(this, ClipboardService::class.java))
            handler.removeCallbacks(updateRunnable)
        } else {
            saveConfig()
            val intent = Intent(this, ClipboardService::class.java).apply {
                putExtra("port", portInput.text.toString().toIntOrNull() ?: 9501)
                putExtra("auth", authInput.text.toString())
                val prefs = getSharedPreferences("config", MODE_PRIVATE)
                putExtra("storageDirUri", prefs.getString("storageDirUri", null))
            }
            startForegroundService(intent)
            handler.postDelayed({
                handler.post(updateRunnable)
            }, 1000)
        }
    }

    private fun loadConfig() {
        val prefs = getSharedPreferences("config", MODE_PRIVATE)
        portInput.setText(prefs.getInt("port", 9501).toString())
        authInput.setText(prefs.getString("auth", ""))

        val storageDirUriString = prefs.getString("storageDirUri", null)
        if (storageDirUriString != null) {
            val uri = Uri.parse(storageDirUriString)
            val resolvedPath = CloudClipboardPaths.getPathFromUri(this, uri)
            storageDirText.text = CloudClipboardPaths.shortDirName(resolvedPath)
            historyFileText.text = CloudClipboardPaths.shortDirName(resolvedPath) + File.separator + "history.json"
        } else {
            val defaultBaseDir = CloudClipboardPaths.resolveBaseDir(this)
            storageDirText.text = CloudClipboardPaths.shortDirName(defaultBaseDir.absolutePath)
            historyFileText.text = CloudClipboardPaths.shortDirName(defaultBaseDir.absolutePath) + File.separator + "history.json"
        }
    }

    private fun saveConfig() {
        val prefs = getSharedPreferences("config", MODE_PRIVATE).edit()
        prefs.putInt("port", portInput.text.toString().toIntOrNull() ?: 9501)
        prefs.putString("auth", authInput.text.toString())
        prefs.apply()
    }

    private fun loadHistory() {
        try {
            val historyFile = File(historyFileText.text.toString())
            val adapter = HistoryAdapter(this, historyFile)
            syncList.adapter = adapter
            syncEmpty.visibility = if (adapter.count == 0) View.VISIBLE else View.GONE
        } catch (e: Exception) {
            syncList.adapter = null
            syncEmpty.visibility = View.VISIBLE
        }
    }

    private fun updateStatus() {
        val hasAuth = authInput.text.toString().isNotEmpty()

        if (ClipboardService.isRunning) {
            statusChip.text = getString(R.string.service_running)
            statusChip.setBackgroundResource(R.drawable.chip_running)
            coverTitle.text = getString(R.string.cover_title_running)
            coverSubtitle.text = getString(R.string.cover_subtitle_running)
            addressText.text = ClipboardService.address
            powerFab.setBackgroundTintList(android.content.res.ColorStateList.valueOf(Color.parseColor("#19A560")))
            powerFab.setImageResource(R.drawable.ic_power)

            if (ClipboardService.address.isNotEmpty()) {
                addressActions.visibility = View.VISIBLE
                val ip = ClipboardService.address
                    .removePrefix("http://")
                    .removePrefix("https://")
                    .substringBefore(":")
                    .ifEmpty { "--" }
                val deviceCount = ClipboardService.onlineDeviceCount.toString()
                val todaySyncs = ClipboardService.todaySyncCount.toString()
                val latency = ClipboardService.averageLatency
                statDevices.text = deviceCount
                statTodaySyncs.text = todaySyncs
                statLatency.text = if (latency >= 0) {
                    String.format("%.1f ms", latency)
                } else {
                    "--"
                }
                infoLan.text = ip
                infoPort.text = portInput.text.toString().takeIf { it.isNotEmpty() } ?: "9501"
                infoAuth.text = getString(if (hasAuth) R.string.stat_auth_on else R.string.stat_auth_off)
                infoDir.text = storageDirText.text.toString()
                generateQr(ClipboardService.address)
            }
        } else {
            statusChip.text = getString(R.string.service_stopped)
            statusChip.setBackgroundResource(R.drawable.chip_stopped)
            coverTitle.text = getString(R.string.cover_title_stopped)
            coverSubtitle.text = getString(R.string.cover_subtitle_stopped)
            addressText.text = "--"
            powerFab.setBackgroundTintList(android.content.res.ColorStateList.valueOf(Color.parseColor("#1677D0")))
            powerFab.setImageResource(R.drawable.ic_power)
            addressActions.visibility = View.GONE
            statDevices.text = "--"
            statTodaySyncs.text = "--"
            statLatency.text = "--"
            qrImage.setImageDrawable(null)
        }
    }

    private fun generateQr(content: String) {
        try {
            val hints = HashMap<EncodeHintType, Any>()
            hints[EncodeHintType.MARGIN] = 0
            hints[EncodeHintType.CHARACTER_SET] = "UTF-8"
            val matrix: BitMatrix = QRCodeWriter().encode(content, BarcodeFormat.QR_CODE, 220, 220, hints)

            val bmp = Bitmap.createBitmap(matrix.width, matrix.height, Bitmap.Config.RGB_565)
            for (x in 0 until matrix.width) {
                for (y in 0 until matrix.height) {
                    bmp.setPixel(x, y, if (matrix.get(x, y)) Color.BLACK else Color.WHITE)
                }
            }
            qrImage.setImageBitmap(bmp)
        } catch (e: Exception) {
            android.util.Log.w("MainActivity", "QR 生成失败", e)
            qrImage.setImageDrawable(null)
        }
    }

    override fun onResume() {
        super.onResume()
        LocalBroadcastManager.getInstance(this).registerReceiver(
            serviceStoppedReceiver,
            IntentFilter(ClipboardService.ACTION_SERVICE_STOPPED)
        )

        updateStatus()
        if (ClipboardService.isRunning) {
            handler.post(updateRunnable)
        }
    }

    override fun onPause() {
        super.onPause()
        LocalBroadcastManager.getInstance(this).unregisterReceiver(serviceStoppedReceiver)
        handler.removeCallbacks(updateRunnable)
    }

    override fun onDestroy() {
        super.onDestroy()
        handler.removeCallbacks(updateRunnable)
    }
}

/** 同步记录适配器: 读取 history.json receive[] 展示 */
class HistoryAdapter(context: Context, historyFile: File) : BaseAdapter() {
    private val items = mutableListOf<Pair<String, String>>() // time, content
    private val inflater = LayoutInflater.from(context)

    init {
        try {
            val text = historyFile.readText()
            val json = JSONObject(text)
            val receive = json.optJSONArray("receive") ?: JSONArray()
            val fmt = SimpleDateFormat("MM-dd HH:mm", Locale.getDefault())
            for (i in receive.length() - 1 downTo 0) {
                val obj = receive.optJSONObject(i) ?: continue
                val type = obj.optString("type")
                if (type != "text") continue
                val ts = obj.optLong("timestamp") * 1000
                val content = obj.optString("content").take(120)
                val room = obj.optString("room")
                val label = if (room.isNotEmpty()) "[$room] $content" else content
                items.add(Pair(fmt.format(Date(ts)), label))
                if (items.size >= 50) break
            }
        } catch (e: Exception) {
            // 文件不存在或解析失败 -> 空列表
        }
    }

    override fun getCount(): Int = items.size
    override fun getItem(position: Int): Any = items[position]
    override fun getItemId(position: Int): Long = position.toLong()

    override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
        val v = convertView ?: inflater.inflate(android.R.layout.simple_list_item_2, parent, false).also {
            it.setPadding(24, 18, 24, 18)
        }
        val (time, content) = items[position]
        val t1 = v.findViewById<TextView>(android.R.id.text1)
        val t2 = v.findViewById<TextView>(android.R.id.text2)
        t1.text = content
        t1.setTextColor(0xFF1A2433.toInt())
        t1.textSize = 14f
        t2.text = time
        t2.setTextColor(0xFF8B93A1.toInt())
        t2.textSize = 11f
        return v
    }
}