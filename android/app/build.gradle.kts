// Top-level build file where you can add configuration options common to all sub-projects/modules.
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.cloudclip"
    compileSdk = 34

    // 1. 添加签名配置
    signingConfigs {
        create("release") {
            // 从命令行 (-P) 或 gradle.properties 文件读取签名信息
            val storeFileProp = project.findProperty("signing.store.file")?.let { file(it) }
            val storePasswordProp = project.findProperty("signing.store.password") as String?
            val keyAliasProp = project.findProperty("signing.key.alias") as String?
            val keyPasswordProp = project.findProperty("signing.key.password") as String?

            // 仅当所有签名信息都存在时才配置签名
            if (storeFileProp != null && storeFileProp.exists() && storePasswordProp != null && keyAliasProp != null && keyPasswordProp != null) {
                // 使用 Kotlin DSL 的赋值语法 (=)
                storeFile = storeFileProp
                storePassword = storePasswordProp
                keyAlias = keyAliasProp
                keyPassword = keyPasswordProp
            }
        }
    }

    defaultConfig {
        applicationId = "com.cloudclip"
        minSdk = 24
        targetSdk = 34
        versionCode = 1
        versionName = "1.0"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            // 将签名配置应用到 release 构建类型
            signingConfig = signingConfigs.getByName("release")
        }
    }

    // 2. 添加 ABI 拆分配置
    splits {
        abi {
            isEnable = true
            reset()
            // 根据传入的 abiFilters 参数来决定包含哪些架构
            val abiFilters = project.findProperty("abiFilters") as String?
            if (abiFilters != null) {
                include(*abiFilters.split(",").toTypedArray())
            } else {
                // 如果没有指定，则包含所有支持的架构
                include("armeabi-v7a", "arm64-v8a", "x86", "x86_64")
            }
            isUniversalApk = false // 不生成通用的 universal APK
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    // AAR 文件
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))

    // Android 基础库
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.appcompat:appcompat:1.6.1")
    implementation("com.google.android.material:material:1.11.0")
    implementation("androidx.constraintlayout:constraintlayout:2.1.4")
}