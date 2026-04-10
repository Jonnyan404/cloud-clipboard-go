package com.cloudclip

import android.app.*
import android.content.Context
import android.content.Intent
import android.net.wifi.WifiManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import androidx.core.app.NotificationCompat
import androidx.localbroadcastmanager.content.LocalBroadcastManager
import mobile.Mobile
import java.io.File
import java.net.Inet4Address
import java.net.NetworkInterface

class ClipboardService : Service() {
    private var service: mobile.Service? = null
    private val handler = Handler(Looper.getMainLooper())
    
    companion object {
        @Volatile
        var isRunning = false
        
        @Volatile
        var address = ""

        const val ACTION_SERVICE_STOPPED = "com.cloudclip.ACTION_SERVICE_STOPPED"
        
        private const val CHANNEL_ID = "CloudClipboardService"
        private const val NOTIFICATION_ID = 1
    }
    
    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
        service = Mobile.newService()
    }
    
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // 立即将服务转为前台，避免 ANR
        val notification = createNotification("正在启动服务...")
        startForeground(NOTIFICATION_ID, notification)

        val port = intent?.getIntExtra("port", 9501) ?: 9501
        val auth = intent?.getStringExtra("auth") ?: ""
        val baseDir = CloudClipboardPaths.resolveBaseDir(this)

        if (!baseDir.exists()) {
            baseDir.mkdirs()
        }
        
        val storageDir = File(baseDir, "uploads").absolutePath
        val historyFile = File(baseDir, "history.json").absolutePath
        val configFile = File(baseDir, "config.json").absolutePath

        // 启动 Go 服务
        Thread {
            try {
                // 这个调用现在会立即返回
                val result = service?.startServer(
                    configFile, // 传递配置文件路径
                    "0.0.0.0",
                    port.toLong(),
                    auth,
                    storageDir,
                    historyFile
                )
                
                // 在主线程上处理结果
                handler.post {
                    if (result.isNullOrEmpty()) {
                        // 启动成功，但服务器仍在后台初始化
                        // 稍作延时，等待服务器完全启动并绑定端口
                        handler.postDelayed({
                            isRunning = true
                            
                            // 从 Go 服务获取地址
                            address = service?.serverAddress ?: ""
                            
                            // 如果 Go 返回空或无效地址, 尝试在 Android 端获取
                            if (address.isEmpty() || address.contains("0.0.0.0")) {
                                address = getLocalAddress(port)
                            }
                            
                            updateNotification("运行中: $address")
                            android.util.Log.d("ClipboardService", "服务启动成功: $address")
                        }, 500) // 延迟 500 毫秒

                    } else {
                        // 如果 StartServer 同步返回错误
                        android.util.Log.e("ClipboardService", "启动失败: $result")
                        updateNotification("启动失败: $result")
                        stopSelf()
                    }
                }
            } catch (e: Exception) {
                android.util.Log.e("ClipboardService", "启动异常", e)
                handler.post {
                    updateNotification("启动异常: ${e.message}")
                    stopSelf()
                }
            }
        }.start()
        
        return START_STICKY
    }
    
    override fun onDestroy() {
        service?.stopServer()
        isRunning = false
        address = ""

        // 发送服务已停止的广播
        val intent = Intent(ACTION_SERVICE_STOPPED)
        LocalBroadcastManager.getInstance(this).sendBroadcast(intent)

        super.onDestroy()
    }
    
    override fun onBind(intent: Intent?): IBinder? = null
    
    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "云剪贴板服务",
                NotificationManager.IMPORTANCE_LOW
            )
            val manager = getSystemService(NotificationManager::class.java)
            manager.createNotificationChannel(channel)
        }
    }
    
    private fun createNotification(text: String): Notification {
        val intent = Intent(this, MainActivity::class.java)
        val pendingIntent = PendingIntent.getActivity(
            this, 0, intent,
            PendingIntent.FLAG_IMMUTABLE
        )
        
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle("云剪贴板")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_menu_share)
            .setContentIntent(pendingIntent)
            .build()
    }
    
    private fun updateNotification(text: String) {
        val manager = getSystemService(NotificationManager::class.java)
        manager.notify(NOTIFICATION_ID, createNotification(text))
    }

    private fun getLocalAddress(port: Int): String {
        return try {
            // 优先使用 WifiManager
            val wifiManager = applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
            val wifiInfo = wifiManager.connectionInfo
            val ipInt = wifiInfo.ipAddress
            
            if (ipInt != 0) {
                val ip = String.format(
                    "%d.%d.%d.%d",
                    ipInt and 0xff,
                    ipInt shr 8 and 0xff,
                    ipInt shr 16 and 0xff,
                    ipInt shr 24 and 0xff
                )
                return "http://$ip:$port"
            }
            
            // WifiManager 失败后，遍历网络接口
            val interfaces = NetworkInterface.getNetworkInterfaces()
            while (interfaces.hasMoreElements()) {
                val networkInterface = interfaces.nextElement()
                val addresses = networkInterface.inetAddresses
                
                while (addresses.hasMoreElements()) {
                    val address = addresses.nextElement()
                    if (!address.isLoopbackAddress && address is Inet4Address) {
                        return "http://${address.hostAddress}:$port"
                    }
                }
            }
            
            "http://0.0.0.0:$port" // 所有方法都失败后的备用地址
        } catch (e: Exception) {
            android.util.Log.e("ClipboardService", "获取IP地址失败", e)
            "http://0.0.0.0:$port"
        }
    }

}