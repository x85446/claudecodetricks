# iOS/Android Build Optimization

## iOS Build Optimization

### Hermes (Required)
```ruby
# ios/Podfile
:hermes_enabled => true
```

### Build Settings
```ruby
# Podfile - Speed up builds
ENV['RCT_NEW_ARCH_ENABLED'] = '1'  # Enable New Architecture

# Release optimizations
installer.pods_project.targets.each do |target|
  target.build_configurations.each do |config|
    config.build_settings['SWIFT_OPTIMIZATION_LEVEL'] = '-O' # Release
    config.build_settings['GCC_OPTIMIZATION_LEVEL'] = 's'    # Size optimize
  end
end
```

### Flipper Removal (Release)
Remove Flipper from release builds to reduce binary size (~3MB):
```ruby
# Podfile - Only include Flipper in debug
if ENV['CONFIGURATION'] == 'Debug'
  use_flipper!()
end
```

## Android Build Optimization

### Hermes (Required)
```groovy
// android/app/build.gradle
project.ext.react = [
    enableHermes: true,
]
```

### ProGuard/R8
```groovy
// android/app/build.gradle
buildTypes {
    release {
        minifyEnabled true
        shrinkResources true
        proguardFiles getDefaultProguardFile('proguard-android-optimize.txt'), 'proguard-rules.pro'
    }
}
```

### ABI Splits (Reduce APK Size)
```groovy
// android/app/build.gradle
splits {
    abi {
        enable true
        reset()
        include "armeabi-v7a", "arm64-v8a", "x86", "x86_64"
        universalApk false
    }
}
```

### App Bundle (AAB)
```bash
# Build AAB instead of APK (Google Play requires this)
cd android && ./gradlew bundleRelease
```

## Build Time Optimization

```bash
# Parallel builds
# android/gradle.properties
org.gradle.parallel=true
org.gradle.daemon=true
org.gradle.jvmargs=-Xmx4g

# iOS - Use ccache
brew install ccache
export CC="ccache clang"
export CXX="ccache clang++"
```

## Clean Build Commands

```bash
# iOS clean
cd ios && xcodebuild clean && pod install --repo-update

# Android clean
cd android && ./gradlew clean

# Full clean (nuclear option)
watchman watch-del-all
rm -rf node_modules && npm install
cd ios && pod install
```
