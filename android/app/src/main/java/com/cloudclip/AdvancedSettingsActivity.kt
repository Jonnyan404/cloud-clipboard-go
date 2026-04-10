package com.cloudclip

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import org.json.JSONException
import org.json.JSONObject
import java.io.File

class AdvancedSettingsActivity : AppCompatActivity() {

    private lateinit var configPathText: TextView
    private lateinit var jsonEditor: EditText
    private lateinit var saveButton: Button
    private lateinit var restoreDefaultsButton: Button
    private lateinit var configFile: File

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_advanced_settings)

        // 初始化视图
        configPathText = findViewById(R.id.configPathText)
        jsonEditor = findViewById(R.id.jsonEditor)
        saveButton = findViewById(R.id.saveButton)
        restoreDefaultsButton = findViewById(R.id.restoreDefaultsButton)

        // 使用与后台服务相同的配置文件路径
        configFile = CloudClipboardPaths.resolveConfigFile(this)
        configPathText.text = "当前配置文件: ${configFile.absolutePath}"

        // 加载当前配置
        loadConfig()

        // 设置按钮监听器
        saveButton.setOnClickListener { saveConfig() }
        restoreDefaultsButton.setOnClickListener { restoreDefaults() }
    }

    private fun loadConfig() {
        try {
            val content = if (configFile.exists()) {
                configFile.readText(Charsets.UTF_8)
            } else {
                getDefaultConfigJson()
            }
            // 格式化JSON以便阅读
            val formattedJson = JSONObject(content).toString(4)
            jsonEditor.setText(formattedJson)
        } catch (e: Exception) {
            jsonEditor.setText("错误: 无法加载或解析配置文件。\n${e.message}")
        }
    }

    private fun saveConfig() {
        val content = jsonEditor.text.toString()
        try {
            // 验证JSON格式是否正确
            JSONObject(content)
            configFile.writeText(content, Charsets.UTF_8)
            Toast.makeText(this, "配置已保存", Toast.LENGTH_SHORT).show()
            finish() // 保存后关闭当前界面
        } catch (e: JSONException) {
            Toast.makeText(this, "保存失败: JSON格式错误", Toast.LENGTH_LONG).show()
        } catch (e: Exception) {
            Toast.makeText(this, "保存失败: ${e.message}", Toast.LENGTH_LONG).show()
        }
    }

    private fun restoreDefaults() {
        try {
            val defaultConfig = getDefaultConfigJson()
            val formattedJson = JSONObject(defaultConfig).toString(4)
            jsonEditor.setText(formattedJson)
            Toast.makeText(this, "已恢复默认配置", Toast.LENGTH_SHORT).show()
        } catch (e: Exception) {
            // Should not happen with default config
        }
    }

    private fun getDefaultConfigJson(): String {
        // 这个默认配置应与 Go 端的 defaultConfig() 保持一致
        return """
        {
            "server": {
                "host": null,
                "port": 9501,
                "prefix": "",
                "history": 100,
                "auth": false,
                "historyFile": null,
                "storageDir": null,
                "roomList": false,
                "roomCleanup": 3600
            },
            "text": {
                "limit": 4096
            },
            "file": {
                "expire": 3600,
                "chunk": 2097152,
                "limit": 268435456
            }
        }
        """.trimIndent()
    }
}