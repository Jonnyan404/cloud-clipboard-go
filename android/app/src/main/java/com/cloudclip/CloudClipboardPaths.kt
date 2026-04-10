package com.cloudclip

import android.content.Context
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.DocumentsContract
import java.io.File

object CloudClipboardPaths {
    private const val PREFS_NAME = "config"
    private const val STORAGE_DIR_URI_KEY = "storageDirUri"
    private const val DEFAULT_FOLDER_NAME = "CloudClipboard"

    fun resolveBaseDir(context: Context): File {
        val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        val storageDirUriString = prefs.getString(STORAGE_DIR_URI_KEY, null)

        if (!storageDirUriString.isNullOrEmpty()) {
            val resolvedPath = getPathFromUri(context, Uri.parse(storageDirUriString))
            if (!resolvedPath.isNullOrEmpty() && resolvedPath != "未知路径") {
                return File(resolvedPath)
            }
        }

        val appDocumentsDir = context.getExternalFilesDir(Environment.DIRECTORY_DOCUMENTS) ?: context.filesDir
        return File(appDocumentsDir, DEFAULT_FOLDER_NAME)
    }

    fun resolveConfigFile(context: Context): File {
        val baseDir = resolveBaseDir(context)
        if (!baseDir.exists()) {
            baseDir.mkdirs()
        }
        return File(baseDir, "config.json")
    }

    fun getPathFromUri(context: Context, uri: Uri): String {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP && DocumentsContract.isTreeUri(uri)) {
            val treeDocId = DocumentsContract.getTreeDocumentId(uri)
            val split = treeDocId.split(":")
            if (split.size > 1) {
                val type = split[0]
                val path = split[1]
                if ("primary".equals(type, ignoreCase = true)) {
                    return "${Environment.getExternalStorageDirectory()}/$path"
                }
            }
        }

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
}