# Cove knowledge upload fixture

This synthetic document exists only for the isolated Cove mobile end-to-end test.

## Observable content

- Fixture marker: `cove-native-knowledge-upload-fixture-v1`
- The document contains no credentials, personal data, or external provider content.
- The native App must select this file through the iOS document picker.
- Cove then uploads, parses, chunks, and marks the document ready.

## Deterministic paragraph

Cove turns uploaded reference material into searchable knowledge. The test verifies the real native selection boundary, the authenticated multipart request, asynchronous ingestion status, and the final non-zero chunk count shown in the App. The fixture is intentionally small so a disposable local worker can process it within a finite test deadline.
