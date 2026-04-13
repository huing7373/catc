buildscript {
    repositories {
        mavenCentral()
        gradlePluginPortal()
    }
    dependencies {
        classpath("org.jreleaser:jreleaser-gradle-plugin:1.18.0")
    }
}


// Top-level build file where you can add configuration options common to all sub-projects/modules.
plugins {
    alias(libs.plugins.androidApplication) apply false
    alias(libs.plugins.jetbrainsKotlinAndroid) apply false
    alias(libs.plugins.androidLibrary) apply false
    id("org.jreleaser") version "1.17.0" apply false
}

// Read version from spine-libgdx gradle.properties to ensure consistency
fun getSpineVersion(): String {
    val spineLibgdxPropsFile = File(project.projectDir, "../spine-libgdx/gradle.properties")
    if (!spineLibgdxPropsFile.exists()) {
        throw GradleException("spine-libgdx gradle.properties file not found at ${spineLibgdxPropsFile.absolutePath}")
    }
    val lines = spineLibgdxPropsFile.readLines()
    for (line in lines) {
        if (line.startsWith("version=")) {
            return line.split("=")[1].trim()
        }
    }
    throw GradleException("version property not found in spine-libgdx gradle.properties")
}

// Placeholder functions - you need to implement these correctly!
fun getRepositoryUsername(): String {
    // Example: return project.findProperty("mavenCentralUsername") as? String ?: System.getenv("MAVEN_CENTRAL_USERNAME") ?: ""
    return project.findProperty("sonatypeUsername") as? String ?: System.getenv("SONATYPE_USERNAME") ?: throw GradleException("Sonatype username not set")
}

fun getRepositoryPassword(): String {
    // Example: return project.findProperty("mavenCentralPassword") as? String ?: System.getenv("MAVEN_CENTRAL_PASSWORD") ?: ""
    return project.findProperty("sonatypePassword") as? String ?: System.getenv("SONATYPE_PASSWORD") ?: throw GradleException("Sonatype password not set")
}

allprojects {
    group = "com.esotericsoftware.spine"
    version = getSpineVersion()
}

// Add a clean task for JReleaser
tasks.register("clean") {
    description = "Clean root project"
}

