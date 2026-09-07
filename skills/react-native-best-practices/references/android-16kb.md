# Android 16KB Page Alignment

## Background

Android 15+ supports 16KB memory pages (up from 4KB). Apps must ensure native libraries are aligned to 16KB boundaries for compatibility.

## Check Compliance

```bash
# Check if native libraries are 16KB aligned
cd android
./gradlew :app:check16KBAlignment

# Or manually check .so files
python3 -c "
import struct, sys, os
for root, dirs, files in os.walk('app/build/intermediates/stripped_native_libs'):
    for f in files:
        if f.endswith('.so'):
            path = os.path.join(root, f)
            with open(path, 'rb') as elf:
                elf.seek(52)  # e_phoff for 32-bit
                print(f'{f}: check alignment manually')
"
```

## Fix Alignment

### Gradle Configuration
```groovy
// android/app/build.gradle
android {
    packagingOptions {
        jniLibs {
            // Ensure 16KB page alignment
            keepDebugSymbols += ['**/*.so']
        }
    }
}
```

### NDK Build
```cmake
# CMakeLists.txt
set(CMAKE_SHARED_LINKER_FLAGS "${CMAKE_SHARED_LINKER_FLAGS} -Wl,-z,max-page-size=16384")
```

## Impact on React Native

- Hermes engine .so files must be aligned
- react-native-reanimated native code must be aligned
- Any library with JNI native code needs verification
- Usually handled by library maintainers - update to latest versions

## When This Matters

- Targeting Android 15+ (API 35+)
- Publishing to Google Play (will be required)
- Using custom native modules with .so files
