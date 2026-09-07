# Image, Font & Resource Optimization

## Image Optimization

### Format Selection
| Format | Use Case | Size | Quality |
|--------|----------|------|---------|
| WebP | Photos, complex images | Smallest | Good |
| PNG | Icons with transparency | Medium | Lossless |
| SVG | Scalable icons, logos | Tiny | Perfect |
| JPEG | Photos (no transparency) | Small | Good |

### Resolution Variants
```
assets/images/
├── logo.png       # @1x (base)
├── logo@2x.png    # @2x (retina)
└── logo@3x.png    # @3x (super retina)
```

React Native automatically selects the right variant.

### Network Images (FastImage)
```typescript
import FastImage from 'react-native-fast-image';

// ALWAYS specify dimensions
<FastImage
  source={{
    uri: imageUrl,
    priority: FastImage.priority.normal,
    cache: FastImage.cacheControl.immutable,
  }}
  style={{ width: 200, height: 200 }}
  resizeMode={FastImage.resizeMode.cover}
/>

// Preload critical images
FastImage.preload([
  { uri: 'https://example.com/hero.webp' },
  { uri: 'https://example.com/avatar.webp' },
]);
```

### Image Compression Script
```bash
# Install cwebp for WebP conversion
brew install webp

# Convert PNGs to WebP
for f in assets/images/*.png; do
  cwebp -q 80 "$f" -o "${f%.png}.webp"
done

# Check savings
ls -la assets/images/*.png assets/images/*.webp
```

## Font Optimization

### Only Include Used Weights
```
assets/fonts/
├── Inter-Regular.ttf     # 400
├── Inter-Medium.ttf      # 500
├── Inter-SemiBold.ttf    # 600
└── Inter-Bold.ttf        # 700
# DON'T include: Thin, ExtraLight, Light, ExtraBold, Black
```

### Font Subsetting
```bash
# Install fonttools
pip3 install fonttools brotli

# Subset to Latin characters only (removes CJK, Arabic, etc.)
pyftsubset Inter-Regular.ttf \
  --output-file=Inter-Regular-subset.ttf \
  --unicodes=U+0000-007F,U+00A0-00FF,U+0100-017F \
  --layout-features='*'
```

### React Native Font Config
```javascript
// react-native.config.js
module.exports = {
  assets: ['./assets/fonts/'],
};
```

```bash
# Link fonts
npx react-native-asset
```

## SVG Icons

```typescript
// Use react-native-svg for scalable icons
import { SvgXml } from 'react-native-svg';

// Or better: pre-process SVGs to components
// npx @svgr/cli --native --typescript icon.svg
```

## Asset Audit

```bash
# Find large assets
find . -path ./node_modules -prune -o \
  \( -name "*.png" -o -name "*.jpg" -o -name "*.gif" \) \
  -exec ls -la {} \; | sort -k5 -rn | head -20

# Total asset size
find assets -type f | xargs du -ch | tail -1
```
