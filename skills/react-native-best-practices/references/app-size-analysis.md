# Total App Size Analysis

## Budget: <40MB Total

| Component | iOS Budget | Android Budget |
|-----------|-----------|---------------|
| JS Bundle (Hermes bytecode) | <3MB | <3MB |
| Native code | <15MB | <10MB |
| Images/Assets | <10MB | <10MB |
| Fonts | <2MB | <2MB |
| Native libraries (.so/.dylib) | <10MB | <15MB |
| **Total** | **<40MB** | **<40MB** |

## Measuring

### iOS
```bash
# Build release archive
xcodebuild -workspace ios/MyApp.xcworkspace \
  -scheme MyApp -configuration Release \
  -archivePath /tmp/MyApp.xcarchive archive

# Check IPA size
# Xcode → Window → Organizer → Select archive → Estimate Size

# Or export and check
xcodebuild -exportArchive -archivePath /tmp/MyApp.xcarchive \
  -exportPath /tmp/MyApp -exportOptionsPlist ExportOptions.plist
ls -lh /tmp/MyApp/*.ipa
```

### Android
```bash
# Build release APK
cd android && ./gradlew assembleRelease

# Check APK size
ls -lh app/build/outputs/apk/release/app-release.apk

# Detailed breakdown
# Android Studio → Build → Analyze APK → Select APK
# Shows: DEX, Native libs, Resources, Assets breakdown
```

## Size Reduction Strategies

### Images
- Use WebP format (30-50% smaller than PNG/JPEG)
- Serve different sizes for different screens (@1x, @2x, @3x)
- Use SVG for icons (react-native-svg)
- Consider loading large images from CDN instead of bundling

### Fonts
- Only include weights you actually use
- Subset fonts to remove unused glyphs
- Consider system fonts for body text

### Native Libraries
- ABI splits on Android (arm64 only for modern devices)
- Remove unused native modules
- Strip debug symbols in release

### Assets
```bash
# Find large assets
find ios android src assets -name "*.png" -o -name "*.jpg" | \
  xargs ls -la | sort -k5 -rn | head -20
```

## App Thinning (iOS)

iOS App Store automatically provides App Thinning:
- Slicing: Device-specific assets
- Bitcode: Server-side optimization
- On-Demand Resources: Download assets when needed

## Google Play Optimization

- Use AAB (Android App Bundle) instead of APK
- Google Play generates optimized APKs per device
- Can reduce download size by 15-30%
