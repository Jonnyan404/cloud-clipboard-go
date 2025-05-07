package com.jonnyan404.cloudclipboard; // 请替换为你的包名

import androidx.appcompat.app.AppCompatActivity;
import android.content.Context;
import android.content.SharedPreferences;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.TextUtils;
import android.util.Log;
import android.view.View;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;
import android.widget.Toast;

import java.io.BufferedReader;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class MainActivity extends AppCompatActivity {

    private static final String TAG = "CloudClipApp";
    private static final String GO_EXECUTABLE_NAME = "cloudclip_android_arm64"; // 确保与assets中的文件名一致
    private static final String PREFS_NAME = "CloudClipPrefs";
    private static final String KEY_HOST = "host";
    private static final String KEY_PORT = "port";
    private static final String KEY_AUTH_PASSWORD = "auth_password";

    private Button buttonStart, buttonStop, buttonSaveSettings, buttonClearLog;
    private EditText editTextHost, editTextPort, editTextAuthPassword;
    private TextView textViewStatus, textViewLog;

    private Process goProcess;
    private final ExecutorService executorService = Executors.newSingleThreadExecutor();
    private final Handler mainHandler = new Handler(Looper.getMainLooper());
    private SharedPreferences sharedPreferences;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);

        sharedPreferences = getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE);

        buttonStart = findViewById(R.id.buttonStart);
        buttonStop = findViewById(R.id.buttonStop);
        buttonSaveSettings = findViewById(R.id.buttonSaveSettings);
        buttonClearLog = findViewById(R.id.buttonClearLog);

        editTextHost = findViewById(R.id.editTextHost);
        editTextPort = findViewById(R.id.editTextPort);
        editTextAuthPassword = findViewById(R.id.editTextAuthPassword);

        textViewStatus = findViewById(R.id.textViewStatus);
        textViewLog = findViewById(R.id.textViewLog);

        loadSettings();
        updateButtonStates();

        buttonStart.setOnClickListener(v -> executorService.execute(this::startGoServiceWrapper));
        buttonStop.setOnClickListener(v -> executorService.execute(this::stopGoServiceWrapper));
        buttonSaveSettings.setOnClickListener(v -> saveSettings());
        buttonClearLog.setOnClickListener(v -> mainHandler.post(() -> textViewLog.setText("")));

    }

    private void loadSettings() {
        editTextHost.setText(sharedPreferences.getString(KEY_HOST, "0.0.0.0"));
        editTextPort.setText(sharedPreferences.getString(KEY_PORT, "9501"));
        editTextAuthPassword.setText(sharedPreferences.getString(KEY_AUTH_PASSWORD, ""));
    }

    private void saveSettings() {
        SharedPreferences.Editor editor = sharedPreferences.edit();
        editor.putString(KEY_HOST, editTextHost.getText().toString().trim());
        editor.putString(KEY_PORT, editTextPort.getText().toString().trim());
        editor.putString(KEY_AUTH_PASSWORD, editTextAuthPassword.getText().toString().trim());
        editor.apply();
        mainHandler.post(() -> Toast.makeText(this, "设置已保存", Toast.LENGTH_SHORT).show());
        // 如果服务正在运行，提示用户可能需要重启服务以应用新设置
        if (goProcess != null && goProcess.isAlive()) {
             mainHandler.post(() -> Toast.makeText(this, "部分设置需要重启服务后生效", Toast.LENGTH_LONG).show());
        }
    }


    private String getGoExecutablePath() {
        return new File(getFilesDir(), GO_EXECUTABLE_NAME).getAbsolutePath();
    }

    private boolean copyAssetToFilesDir(String assetName, File targetFile) {
        // 优化：如果文件已存在且大小一致（或基于版本号），则不复制
        if (targetFile.exists()) {
            Log.d(TAG, "可执行文件已存在: " + targetFile.getAbsolutePath());
            // return true; // 假设已存在的是正确的
        }
        try (InputStream inputStream = getAssets().open(assetName);
             OutputStream outputStream = new FileOutputStream(targetFile)) {
            byte[] buffer = new byte[1024 * 4]; // 4KB buffer
            int read;
            while ((read = inputStream.read(buffer)) != -1) {
                outputStream.write(buffer, 0, read);
            }
            Log.d(TAG, "已复制 " + assetName + " 到 " + targetFile.getAbsolutePath());
            return true;
        } catch (IOException e) {
            Log.e(TAG, "复制 asset 失败: " + assetName, e);
            mainHandler.post(() -> appendLog("错误: 复制可执行文件失败: " + e.getMessage()));
            return false;
        }
    }

    private void startGoServiceWrapper() {
        if (goProcess != null && goProcess.isAlive()) {
            mainHandler.post(() -> {
                updateStatus("服务已经在运行中。");
                Toast.makeText(MainActivity.this, "服务已经在运行中", Toast.LENGTH_SHORT).show();
            });
            Log.d(TAG, "Go process is already running.");
            return;
        }

        mainHandler.post(() -> updateStatus("正在准备启动服务..."));
        File goExecutable = new File(getGoExecutablePath());

        if (!copyAssetToFilesDir(GO_EXECUTABLE_NAME, goExecutable)) {
            mainHandler.post(() -> updateStatus("启动失败: 无法准备可执行文件。"));
            return;
        }

        if (!goExecutable.exists()) {
            mainHandler.post(() -> updateStatus("错误：找不到 Go 可执行文件。"));
            Log.e(TAG, "Go executable not found after copy attempt.");
            return;
        }

        if (!goExecutable.canExecute()) {
            if (!goExecutable.setExecutable(true, true)) {
                mainHandler.post(() -> updateStatus("错误：无法设置可执行权限。"));
                Log.e(TAG, "Failed to set executable permission.");
                return;
            }
            Log.d(TAG, "已为 " + goExecutable.getName() + " 设置可执行权限");
        }

        File internalStorageDir = getFilesDir();
        String storagePath = new File(internalStorageDir, "clipboard_data").getAbsolutePath();
        if (!new File(storagePath).mkdirs()) {
            Log.w(TAG, "创建存储目录可能失败 (或已存在): " + storagePath);
        }
        String historyFilePath = new File(internalStorageDir, "history.json").getAbsolutePath();

        List<String> command = new ArrayList<>();
        command.add(goExecutable.getAbsolutePath());

        String host = editTextHost.getText().toString().trim();
        if (!TextUtils.isEmpty(host)) command.add("-host=" + host);

        String port = editTextPort.getText().toString().trim();
        if (!TextUtils.isEmpty(port)) command.add("-port=" + port);

        String authPassword = editTextAuthPassword.getText().toString().trim();
        if (!TextUtils.isEmpty(authPassword)) command.add("-auth=" + authPassword);

        command.add("-storage=" + storagePath);
        command.add("-historyfile=" + historyFilePath);
        // 你可以添加其他必要的默认参数，例如 -config 如果你需要一个默认配置文件

        ProcessBuilder processBuilder = new ProcessBuilder(command);
        processBuilder.redirectErrorStream(true); // 合并 stdout 和 stderr
        // processBuilder.directory(getFilesDir()); // 设置工作目录为应用文件目录

        Log.d(TAG, "准备启动 Go 服务，命令: " + TextUtils.join(" ", command));
        mainHandler.post(() -> {
            updateStatus("正在启动服务...");
            appendLog("CMD: " + TextUtils.join(" ", command));
        });


        try {
            goProcess = processBuilder.start();
            mainHandler.post(() -> {
                updateStatus("服务已启动。");
                updateButtonStates();
                Toast.makeText(MainActivity.this, "服务已启动", Toast.LENGTH_SHORT).show();
            });
            Log.d(TAG, "Go 服务已启动。PID: " + goProcess.toString()); // toString() gives process info

            // 异步读取 Go 程序的输出
            BufferedReader reader = new BufferedReader(new InputStreamReader(goProcess.getInputStream()));
            String line;
            while ((line = reader.readLine()) != null) {
                final String logLine = line;
                Log.i(TAG + "-GoOutput", logLine);
                mainHandler.post(() -> appendLog(logLine));
            }
            reader.close(); // 关闭reader

        } catch (IOException e) {
            Log.e(TAG, "启动 Go 服务失败", e);
            final String errorMsg = e.getMessage();
            mainHandler.post(() -> {
                updateStatus("启动服务失败: " + errorMsg);
                appendLog("错误: 启动服务失败 - " + errorMsg);
                updateButtonStates();
            });
            goProcess = null; // 确保进程对象为空
        } finally {
            // 当进程的输出流结束时（通常意味着进程已终止）
            if (goProcess != null) {
                int exitCode = -1;
                try {
                    // 确保进程真的结束了
                    if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
                        if(goProcess.isAlive()) goProcess.waitFor(); // 等待进程结束
                        exitCode = goProcess.exitValue();
                    } else {
                        // 对于旧版本，isAlive 可能不可靠，我们只能假设它结束了
                         // 或者使用轮询检查 isAlive (不推荐)
                    }
                } catch (IllegalThreadStateException itse) {
                    // 进程仍在运行，这不应该在 finally 的这个点发生，除非是异步读取结束但进程未结束
                     Log.w(TAG, "进程仍在运行，但输出流已关闭？");
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                }
                final int finalExitCode = exitCode;
                Log.d(TAG, "Go 进程已结束。退出码: " + finalExitCode);
                mainHandler.post(() -> {
                    updateStatus("服务已停止。退出码: " + finalExitCode);
                    appendLog("服务已停止。退出码: " + finalExitCode);
                    updateButtonStates();
                });
                goProcess = null;
            }
        }
    }

    private void stopGoServiceWrapper() {
        if (goProcess != null && goProcess.isAlive()) {
            mainHandler.post(() -> {
                updateStatus("正在停止服务...");
                Toast.makeText(MainActivity.this, "正在停止服务...", Toast.LENGTH_SHORT).show();
            });
            Log.d(TAG, "正在停止 Go 服务...");

            goProcess.destroy(); // 发送 SIGTERM
            try {
                // 等待一段时间确保进程关闭，或者使用更复杂的逻辑
                if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
                    goProcess.waitFor(5, java.util.concurrent.TimeUnit.SECONDS); // 等待5秒
                } else {
                    // 旧版本可能需要不同的处理方式
                    Thread.sleep(1000); // 简单等待
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                Log.e(TAG, "等待 Go 进程停止时被中断", e);
            }

            if (goProcess != null && goProcess.isAlive()) {
                Log.w(TAG, "Go 进程在 destroy 后仍然存活，尝试强制停止 (destroyForcibly)");
                if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
                    goProcess.destroyForcibly(); // 发送 SIGKILL
                }
            }
            goProcess = null; // 清理引用
            Log.d(TAG, "Go 服务已请求停止。");
             mainHandler.post(() -> {
                updateStatus("服务已停止。");
                updateButtonStates();
            });
        } else {
            mainHandler.post(() -> {
                updateStatus("服务未运行。");
                Toast.makeText(MainActivity.this, "服务未运行", Toast.LENGTH_SHORT).show();
            });
            Log.d(TAG, "Go process is not running or already stopped.");
        }
    }

    private void updateStatus(String message) {
        if (textViewStatus != null) {
            textViewStatus.setText("状态: " + message);
        }
    }

    private void appendLog(String message) {
        if (textViewLog != null) {
            textViewLog.append(message + "\n");
            // 自动滚动到底部 (可选)
            final ScrollView scrollView = findViewById(R.id.logScrollView); // 假设你的TextView在ScrollView内
            if (scrollView != null) {
                scrollView.post(() -> scrollView.fullScroll(View.FOCUS_DOWN));
            }
        }
    }

    private void updateButtonStates() {
        boolean isRunning = (goProcess != null && goProcess.isAlive());
        buttonStart.setEnabled(!isRunning);
        buttonStop.setEnabled(isRunning);
        // 可以在服务运行时禁用设置编辑，或提示重启生效
        editTextHost.setEnabled(!isRunning);
        editTextPort.setEnabled(!isRunning);
        editTextAuthPassword.setEnabled(!isRunning);
        buttonSaveSettings.setEnabled(!isRunning);
    }


    @Override
    protected void onDestroy() {
        super.onDestroy();
        Log.d(TAG, "MainActivity onDestroy, 尝试停止服务...");
        executorService.execute(this::stopGoServiceWrapper); // 确保在后台线程停止
        executorService.shutdown(); // 关闭线程池
    }
}