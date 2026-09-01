# Native SDKs vs JS Polyfills

## Rule of Thumb

**Prefer native SDKs when:**
- Performance-critical operations (crypto, image processing, video)
- Platform-specific capabilities (biometrics, NFC, Bluetooth)
- Hardware access (camera, GPS, sensors)
- Background processing

**JS polyfills are acceptable when:**
- Simple utilities (date formatting, string manipulation)
- The operation is infrequent (not hot path)
- Bundle size is a concern and native overhead isn't justified
- Cross-platform consistency matters more than performance

## Common Decisions

| Task | Native SDK | JS Polyfill | Recommendation |
|------|-----------|-------------|----------------|
| Crypto/Hashing | react-native-crypto | crypto-js | **Native** - 10-100x faster |
| Image resize | react-native-image-resizer | sharp (JS) | **Native** - GPU-accelerated |
| Date formatting | Hermes built-in Intl | date-fns | **JS** - Good enough, smaller |
| JSON parsing | N/A | Built-in | **JS** - Already optimized |
| SQLite | react-native-sqlite-storage | sql.js (WASM) | **Native** - Direct file I/O |
| Keychain/Keystore | react-native-keychain | N/A | **Native** - Only option |
| Biometrics | react-native-biometrics | N/A | **Native** - Only option |
| HTTP client | NSURLSession/OkHttp (via RN) | axios/fetch | **JS** - fetch is already native |
| Storage | react-native-mmkv | AsyncStorage | **Native (MMKV)** - 30x faster |
| PDF generation | react-native-pdf-lib | pdfmake | **Native** - Memory efficient |

## Hermes Intl Support

Hermes has limited Intl support. Check what's available:
- `Intl.NumberFormat` ✅ (basic)
- `Intl.DateTimeFormat` ✅ (basic)
- `Intl.Collator` ✅
- Full ICU data: Enable in `react-native.config.js`

```javascript
// react-native.config.js
module.exports = {
  // Enable full Intl support (increases bundle ~6MB)
  dependencies: {},
  project: {
    ios: { hermes_enabled: true },
    android: { hermes_enabled: true },
  },
};
```

## Bundle Impact Assessment

Before adding a native dependency:
1. Check if JS alternative adds <50KB to bundle
2. If yes and not performance-critical, prefer JS
3. If native, verify New Architecture compatibility
4. Check maintenance status (last commit, open issues)
