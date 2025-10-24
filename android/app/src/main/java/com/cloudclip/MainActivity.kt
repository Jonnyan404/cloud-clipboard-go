package com.cloudclip

import android.Manifest
import android.app.Activity
import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.os.Handler
import android.os.Looper
import android.provider.DocumentsContract
import android.view.View
import android.widget.*
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.localbroadcastmanager.content.LocalBroadcastManager
import java.io.File

class MainActivity : AppCompatActivity() {
    private lateinit var statusText: TextView
    private lateinit var addressText: TextView
    private lateinit var portInput: EditText
    private lateinit var authInput: EditText
    private lateinit var startStopButton: Button
    private lateinit var advancedSettingsButton: Button
    private lateinit var storageDirText: TextView
    private lateinit var historyFileText: TextView
    private lateinit var addressLayout: View
    private lateinit var openBrowserButton: ImageButton
    private lateinit var copyAddressButton: ImageButton
    private lateinit var githubButton: ImageButton

    private val handler = Handler(Looper.getMainLooper())
    private val updateRunnable = object : Runnable {
        override fun run() {
            updateStatus()
            if (ClipboardService.isRunning) {
                // 如果服务正在运行,继续定期更新
                handler.postDelayed(this, 1000)
            }
        }
    }

    // 创建广播接收器
    private val serviceStoppedReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action == ClipboardService.ACTION_SERVICE_STOPPED) {
                // 收到服务停止的广播后，更新UI
                updateStatus()
            }
        }
    }

    companion object {
        private const val STORAGE_PERMISSION_CODE = 101
    }

    // 注册目录选择器回调
    private val openDirectoryLauncher = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            result.data?.data?.also { uri ->
                // 获得持久化权限
                contentResolver.takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_GRANT_WRITE_URI_PERMISSION)
                
                // 保存并显示路径
                val prefs = getSharedPreferences("config", MODE_PRIVATE).edit()
                prefs.putString("storageDirUri", uri.toString())
                prefs.apply()
                storageDirText.text = getPathFromUri(this, uri)
            }
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusText = findViewById(R.id.statusText)
        addressText = findViewById(R.id.addressText)
        portInput = findViewById(R.id.portInput)
        authInput = findViewById(R.id.authInput)
        startStopButton = findViewById(R.id.startStopButton)
        advancedSettingsButton = findViewById(R.id.advancedSettingsButton)
        storageDirText = findViewById(R.id.storageDirText)
        historyFileText = findViewById(R.id.historyFileText)
        addressLayout = findViewById(R.id.addressLayout)
        openBrowserButton = findViewById(R.id.openBrowserButton)
        copyAddressButton = findViewById(R.id.copyAddressButton)
        githubButton = findViewById(R.id.githubButton)

        loadConfig()
        
        // 检查并请求存储权限
        checkAndRequestPermissions()

        storageDirText.setOnClickListener {
            val intent = Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
            openDirectoryLauncher.launch(intent)
        }
        // 历史文件将存储在存储目录中，因此其选择器与存储目录联动
        historyFileText.setOnClickListener { storageDirText.performClick() }

        startStopButton.setOnClickListener {
            if (ClipboardService.isRunning) {
                // Stop the service
                stopService(Intent(this, ClipboardService::class.java))
                handler.removeCallbacks(updateRunnable) // 停止轮询
                // 不再需要立即调用 updateStatus()，等待广播通知
                // updateStatus() 
            } else {
                // Start the service
                saveConfig() // 保存当前配置
                val intent = Intent(this, ClipboardService::class.java).apply {
                    putExtra("port", portInput.text.toString().toIntOrNull() ?: 9501)
                    putExtra("auth", authInput.text.toString())
                    // 传递URI字符串
                    val prefs = getSharedPreferences("config", MODE_PRIVATE)
                    putExtra("storageDirUri", prefs.getString("storageDirUri", null))
                }
                startForegroundService(intent)
                
                // 延迟后开始检查状态，给服务启动时间
                handler.postDelayed({
                    handler.post(updateRunnable) // 启动轮询
                }, 1000) // 1秒延迟足够
            }
        }

        advancedSettingsButton.setOnClickListener {
            val intent = Intent(this, AdvancedSettingsActivity::class.java)
            startActivity(intent)
        }

        githubButton.setOnClickListener {
            val intent = Intent(Intent.ACTION_VIEW, Uri.parse("https://github.com/Jonnyan404/cloud-clipboard-go"))
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
                Toast.makeText(this, "地址已复制", Toast.LENGTH_SHORT).show()
            }
        }
    }

    private fun checkAndRequestPermissions() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED) {
                ActivityCompat.requestPermissions(
                    this,
                    arrayOf(Manifest.permission.WRITE_EXTERNAL_STORAGE),
                    STORAGE_PERMISSION_CODE
                )
            }
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == STORAGE_PERMISSION_CODE) {
            if (!(grantResults.isNotEmpty() && grantResults[0] == PackageManager.PERMISSION_GRANTED)) {
                Toast.makeText(this, "存储权限被拒绝，文件将无法持久化保存！", Toast.LENGTH_LONG).show()
            }
        }
    }

    private fun loadConfig() {
        val prefs = getSharedPreferences("config", MODE_PRIVATE)
        portInput.setText(prefs.getInt("port", 9501).toString())
        authInput.setText(prefs.getString("auth", ""))

        // 加载并显示持久化的路径
        val storageDirUriString = prefs.getString("storageDirUri", null)
        if (storageDirUriString != null) {
            val uri = Uri.parse(storageDirUriString)
            storageDirText.text = getPathFromUri(this, uri)
            historyFileText.text = getPathFromUri(this, uri) + File.separator + "history.json"
        } else {
            storageDirText.text = "点击选择目录 (推荐 Documents/CloudClipboard)"
            historyFileText.text = "将与存储目录同步"
        }
    }

    private fun saveConfig() {
        val prefs = getSharedPreferences("config", MODE_PRIVATE).edit()
        prefs.putInt("port", portInput.text.toString().toIntOrNull() ?: 9501)
        prefs.putString("auth", authInput.text.toString())
        // 路径URI已在选择时保存，此处无需重复保存
        prefs.apply()
    }

    // 辅助函数：从URI获取一个可读的路径用于显示
    private fun getPathFromUri(context: Context, uri: Uri): String {
        // 尝试从 Document Provider 获取路径
        if (DocumentsContract.isDocumentUri(context, uri)) {
            val docId = DocumentsContract.getTreeDocumentId(uri)
            val split = docId.split(":")
            if (split.size > 1) {
                val type = split[0]
                val path = split[1]
                if ("primary".equals(type, ignoreCase = true)) {
                    return "${Environment.getExternalStorageDirectory()}/$path"
                }
            }
        }
        return uri.path ?: "未知路径"
    }

    private fun updateStatus() {
        if (ClipboardService.isRunning) {
            statusText.text = getString(R.string.service_running)
            addressText.text = ClipboardService.address
            startStopButton.text = getString(R.string.stop_service)
            startStopButton.setBackgroundColor(Color.parseColor("#FF4081")) // 红色系
            
            if (ClipboardService.address.isNotEmpty()) {
                addressLayout.visibility = View.VISIBLE
            }

        } else {
            statusText.text = getString(R.string.service_stopped)
            addressText.text = ""
            startStopButton.text = getString(R.string.start_service)
            startStopButton.setBackgroundColor(Color.parseColor("#3F51B5")) // 蓝色系
            addressLayout.visibility = View.GONE
        }
    }

    override fun onResume() {
        super.onResume()
        // 注册广播接收器
        LocalBroadcastManager.getInstance(this).registerReceiver(
            serviceStoppedReceiver,
            IntentFilter(ClipboardService.ACTION_SERVICE_STOPPED)
        )

        updateStatus()
        // 如果服务正在运行,启动定期更新
        if (ClipboardService.isRunning) {
            handler.post(updateRunnable)
        }
    }
    
    override fun onPause() {
        super.onPause()
        // 注销广播接收器
        LocalBroadcastManager.getInstance(this).unregisterReceiver(serviceStoppedReceiver)
        // 停止定期更新
        handler.removeCallbacks(updateRunnable)
    }
    
    override fun onDestroy() {
        super.onDestroy()
        handler.removeCallbacks(updateRunnable)
    }
}