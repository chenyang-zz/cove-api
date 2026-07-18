import { useColorScheme } from 'react-native';

export type Palette = {
  page: string;
  surface: string;
  input: string;
  surfaceMuted: string;
  text: string;
  textSecondary: string;
  textMuted: string;
  border: string;
  borderStrong: string;
  accent: string;
  accentPressed: string;
  accentText: string;
  danger: string;
  dangerSurface: string;
  shadow: string;
  scrim: string;
  homeIndicator: string;
};

export const lightPalette: Palette = {
  page: '#F1F8F8',
  surface: '#FBFEFE',
  input: '#F6FBFB',
  surfaceMuted: '#E0EFF0',
  text: '#10272C',
  textSecondary: '#4F6A70',
  textMuted: '#6B8589',
  border: '#C8DFE0',
  borderStrong: '#97B9BC',
  accent: '#177D81',
  accentPressed: '#126A6E',
  accentText: '#F3FFFF',
  danger: '#AE3934',
  dangerSurface: '#FBEDEC',
  shadow: '#1F5963',
  scrim: 'rgba(7, 28, 33, 0.36)',
  homeIndicator: '#000000',
};

export const darkPalette: Palette = {
  page: '#071A21',
  surface: '#0B2229',
  input: '#0D272F',
  surfaceMuted: '#12323A',
  text: '#EDF8F8',
  textSecondary: '#B6C9CC',
  textMuted: '#8FA8AC',
  border: '#29474E',
  borderStrong: '#41666D',
  accent: '#45C8C1',
  accentPressed: '#34B2AC',
  accentText: '#062126',
  danger: '#FF9A96',
  dangerSurface: '#3B2226',
  shadow: '#000000',
  scrim: 'rgba(0, 0, 0, 0.58)',
  homeIndicator: '#FFFFFF',
};

export function usePalette(): Palette {
  return useColorScheme() === 'dark' ? darkPalette : lightPalette;
}
