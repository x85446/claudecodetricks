# R8/ProGuard Android Optimization

## What R8 Does

R8 (successor to ProGuard) optimizes Android builds:
- **Code shrinking** - Removes unused classes and methods
- **Code optimization** - Inlines methods, removes dead branches
- **Obfuscation** - Shortens class/method names (smaller DEX)
- **Resource shrinking** - Removes unused resources

## Configuration

```groovy
// android/app/build.gradle
android {
    buildTypes {
        release {
            minifyEnabled true          // Enable R8
            shrinkResources true        // Remove unused resources
            proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'),
                         'proguard-rules.pro'
        }
    }
}
```

## ProGuard Rules for React Native

```proguard
# android/app/proguard-rules.pro

# React Native
-keep class com.facebook.react.** { *; }
-keep class com.facebook.hermes.** { *; }
-keep class com.facebook.jni.** { *; }

# Hermes
-keep class com.facebook.hermes.unicode.** { *; }
-keep class com.facebook.jni.** { *; }

# react-native-reanimated
-keep class com.swmansion.reanimated.** { *; }

# react-native-gesture-handler
-keep class com.swmansion.gesturehandler.** { *; }

# react-native-screens
-keep class com.swmansion.rnscreens.** { *; }

# react-native-svg
-keep class com.horcrux.svg.** { *; }

# OkHttp (used by RN networking)
-dontwarn okhttp3.**
-keep class okhttp3.** { *; }
-keep interface okhttp3.** { *; }

# Prevent R8 from removing native methods
-keepclassmembers class * {
    @com.facebook.react.bridge.ReactMethod *;
    @com.facebook.react.uimanager.annotations.ReactProp *;
}
```

## Debugging R8 Issues

```bash
# Build with R8 logging
cd android && ./gradlew assembleRelease --info 2>&1 | grep -i "r8\|proguard"

# If crash in release but not debug, likely R8 removed needed code
# Add -keep rule for the affected class
```

## Typical Size Impact

| Without R8 | With R8 | Savings |
|-----------|---------|---------|
| ~25MB APK | ~18MB APK | ~28% |
| ~8MB DEX | ~5MB DEX | ~37% |
