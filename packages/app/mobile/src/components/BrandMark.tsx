import { Image, View } from 'react-native';

const brandSymbol = require('../../assets/images/brand-symbol.png');

export function BrandMark({ size = 36 }: { size?: number }) {
  const markSize = size * (5 / 6);

  return (
    <View style={{ width: size, height: size, alignItems: 'center', justifyContent: 'center' }}>
      <Image
        accessibilityIgnoresInvertColors
        source={brandSymbol}
        resizeMode="contain"
        style={{ width: markSize, height: markSize }}
      />
    </View>
  );
}
