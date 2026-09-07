# Advanced Metro Configuration

## Default Config Setup

```javascript
// metro.config.js
const { getDefaultConfig, mergeConfig } = require('@react-native/metro-config');

const defaultConfig = getDefaultConfig(__dirname);

const config = {
  transformer: {
    getTransformOptions: async () => ({
      transform: {
        experimentalImportSupport: true,
        inlineRequires: true,
        nonInlinedRequires: [
          'React',
          'react',
          'react-native',
          'react/jsx-runtime',
        ],
      },
    }),
    babelTransformerPath: require.resolve('react-native-svg-transformer'),
  },
  resolver: {
    assetExts: defaultConfig.resolver.assetExts.filter(ext => ext !== 'svg'),
    sourceExts: [...defaultConfig.resolver.sourceExts, 'svg'],
    blockList: [
      /.*\/__tests__\/.*/,
      /.*\.test\.[jt]sx?$/,
    ],
  },
  server: {
    port: 8081,
  },
};

module.exports = mergeConfig(defaultConfig, config);
```

## SVG Support

```bash
npm install react-native-svg react-native-svg-transformer
```

Add to metro.config.js as shown above, then:
```typescript
// Import SVGs as components
import Logo from '../assets/logo.svg';
<Logo width={120} height={40} />
```

## Custom Asset Types

```javascript
resolver: {
  assetExts: [...defaultConfig.resolver.assetExts, 'lottie', 'obj', 'mtl'],
}
```

## Troubleshooting Metro

| Problem | Solution |
|---------|----------|
| "Unable to resolve module" | `watchman watch-del-all && npx react-native start --reset-cache` |
| Stale cache | `npx react-native start --reset-cache` |
| Port in use | `lsof -ti:8081 \| xargs kill -9` then restart |
| Slow bundling | Enable `inlineRequires`, check `blockList` |
| Source map issues | Verify Hermes is enabled, rebuild |
| Module not found after install | Restart Metro, clear watchman |
