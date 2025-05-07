// filepath: Android/build.gradle.kts
// Top-level build file where you can add configuration options common to all sub-projects/modules.
plugins {
    id("com.android.application") version "8.2.0" apply false
    // 如果使用 Kotlin
    // id("org.jetbrains.kotlin.android") version "1.9.0" apply false // 示例版本
}
android {
    namespace = "com.yourdomain.cloudclipboard" // 替换为你的实际包名
    compileSdk = 34 // 或者你项目使用的 SDK 版本

    defaultConfig {
        applicationId = "com.jonnyan404.cloudclipboard" // 替换为你的实际应用 ID
        minSdk = 24 // 或者你项目支持的最低 SDK
        targetSdk = 34 // 或者你项目针对的 SDK
        versionCode = 1
        versionName = "1.0"
        // ... 其他配置 ...
    }

    buildTypes {
        getByName("release") {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }
    // ... 其他 android 配置 ...
}