const emojiSequencePattern = /(?:\p{Regional_Indicator}{2}|[#*0-9]\uFE0F?\u20E3|\p{Extended_Pictographic}(?:\uFE0F|\uFE0E)?(?:\p{Emoji_Modifier})?(?:\u200D\p{Extended_Pictographic}(?:\uFE0F|\uFE0E)?(?:\p{Emoji_Modifier})?)*)/gu;

export function stripUnsupportedMarkdownEmoji(markdown: string) {
  return markdown
    .replace(emojiSequencePattern, '')
    .replace(/^(#{1,6})[ \t]{2,}/gm, '$1 ');
}
