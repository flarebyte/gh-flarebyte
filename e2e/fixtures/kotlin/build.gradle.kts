plugins {
    kotlin("jvm") version "2.2.21"
    jacoco
    id("org.jlleitschuh.gradle.ktlint") version "13.1.0"
}

repositories {
    mavenCentral()
}

kotlin {
    jvmToolchain(21)
}

jacoco {
    toolVersion = "0.8.13"
}

dependencies {
    testImplementation(kotlin("test-junit5"))
    testRuntimeOnly("org.junit.jupiter:junit-jupiter-engine:5.12.2")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher:1.12.2")
}

tasks.test {
    useJUnitPlatform()
}

dependencyLocking {
    lockAllConfigurations()
}

// Bootstrap locks for every resolvable configuration, including quality tools.
tasks.register("resolveLocks") {
    doLast {
        configurations.filter { it.isCanBeResolved }.forEach { it.resolve() }
    }
}
