# CI/CD Pipeline Configuration

## Overview

| Stage | iOS | Android |
|-------|-----|---------|
| Lint & Type Check | `npx tsc --noEmit` | Same |
| Unit Tests | `npx jest` | Same |
| Build | Xcode Archive | Gradle Bundle |
| E2E Tests | Detox iOS | Detox Android |
| Deploy Beta | TestFlight | Play Store Beta |
| Deploy Prod | App Store | Play Store |

## GitHub Actions Example

```yaml
# .github/workflows/ci.yml
name: CI

on:
  pull_request:
    branches: [main, develop]

jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
      - run: npm ci
      - run: npx tsc --noEmit
      - run: npx jest --coverage

  build-android:
    needs: lint-and-test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
      - uses: actions/setup-java@v4
        with:
          distribution: zulu
          java-version: 17
      - run: npm ci
      - run: cd android && ./gradlew assembleRelease

  build-ios:
    needs: lint-and-test
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
      - run: npm ci
      - run: cd ios && pod install
      - run: |
          xcodebuild -workspace ios/MyApp.xcworkspace \
            -scheme MyApp -configuration Release \
            -sdk iphonesimulator \
            -destination 'platform=iOS Simulator,name=iPhone 15' \
            build
```

## Fastlane Setup

```ruby
# ios/Gemfile
source "https://rubygems.org"
gem "fastlane"
gem "cocoapods"

# ios/fastlane/Fastfile
default_platform(:ios)

platform :ios do
  desc "Push to TestFlight"
  lane :beta do
    setup_ci if is_ci
    increment_build_number(xcodeproj: "MyApp.xcodeproj")
    build_app(
      workspace: "MyApp.xcworkspace",
      scheme: "MyApp",
      export_method: "app-store"
    )
    upload_to_testflight(skip_waiting_for_build_processing: true)
  end
end
```

```ruby
# android/fastlane/Fastfile
default_platform(:android)

platform :android do
  desc "Deploy to Play Store Beta"
  lane :beta do
    gradle(task: "bundleRelease")
    upload_to_play_store(
      track: "beta",
      aab: "app/build/outputs/bundle/release/app-release.aab"
    )
  end
end
```

## Code Signing

### iOS
- Use `match` (Fastlane) for team certificate management
- Store certificates in private Git repo or encrypted storage
- CI uses match to fetch certs automatically

### Android
- Generate keystore: `keytool -genkeypair ...`
- Store keystore securely (CI secrets, not in repo)
- Reference in `android/app/build.gradle`:
```groovy
signingConfigs {
    release {
        storeFile file(System.getenv('KEYSTORE_PATH') ?: 'release.keystore')
        storePassword System.getenv('KEYSTORE_PASSWORD') ?: ''
        keyAlias System.getenv('KEY_ALIAS') ?: ''
        keyPassword System.getenv('KEY_PASSWORD') ?: ''
    }
}
```

## Environment Variables

```bash
# Use react-native-config for .env management
npm install react-native-config

# .env.staging
API_URL=https://staging-api.gravhl.com
CENTRIFUGO_URL=wss://staging-ws.gravhl.com

# .env.production
API_URL=https://api.gravhl.com
CENTRIFUGO_URL=wss://ws.gravhl.com
```

```typescript
import Config from 'react-native-config';
const apiUrl = Config.API_URL;
```
