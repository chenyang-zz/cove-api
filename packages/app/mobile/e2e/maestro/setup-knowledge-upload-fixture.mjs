const requiredVariables = [
  'MAESTRO_E2E_API_URL',
  'MAESTRO_E2E_LLM_BASE_URL',
  'MAESTRO_E2E_USERNAME',
  'MAESTRO_E2E_EMAIL',
  'MAESTRO_E2E_PASSWORD',
];

for (const name of requiredVariables) {
  if (!process.env[name]?.trim()) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
}

const apiURL = new URL(process.env.MAESTRO_E2E_API_URL);
const localHostnames = new Set(['127.0.0.1', 'localhost', '::1', '[::1]']);
if (!['http:', 'https:'].includes(apiURL.protocol)
  || apiURL.username
  || apiURL.password
  || !localHostnames.has(apiURL.hostname)) {
  throw new Error('MAESTRO_E2E_API_URL must target a loopback, run-owned local API');
}
apiURL.pathname = apiURL.pathname.replace(/\/+$/, '');

const providerURL = new URL(process.env.MAESTRO_E2E_LLM_BASE_URL);
if (!['http:', 'https:'].includes(providerURL.protocol)
  || providerURL.username
  || providerURL.password
  || !localHostnames.has(providerURL.hostname)) {
  throw new Error('MAESTRO_E2E_LLM_BASE_URL must target a loopback, run-owned local provider');
}
providerURL.pathname = providerURL.pathname.replace(/\/+$/, '');

async function readEnvelope(response, operation) {
  const envelope = await response.json().catch(() => null);
  if (!response.ok || !envelope || envelope.code !== 0) {
    const message = envelope?.message || `HTTP ${response.status}`;
    throw new Error(`${operation} failed: ${message}`);
  }
  return envelope;
}

const username = process.env.MAESTRO_E2E_USERNAME.trim();
const registrationResponse = await fetch(new URL('/api/auth/register', apiURL), {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    username,
    email: process.env.MAESTRO_E2E_EMAIL.trim(),
    password: process.env.MAESTRO_E2E_PASSWORD,
  }),
});
const registrationEnvelope = await readEnvelope(registrationResponse, '/api/auth/register');
const accessToken = registrationEnvelope.data?.access_token;

if (!accessToken || registrationEnvelope.data.username !== username) {
  throw new Error('registration did not return the expected disposable user session');
}

const authorization = { Authorization: `Bearer ${accessToken}` };
const modelResponse = await fetch(new URL('/api/model-configs/', apiURL), {
  method: 'POST',
  headers: {
    ...authorization,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    type: 'embedding',
    provider: 'openai',
    name: `Local deterministic embedding ${username}`,
    model_name: 'cove-e2e-embedding',
    api_key: 'cove-e2e-local-key',
    base_url: providerURL.toString().replace(/\/+$/, ''),
    is_default: true,
  }),
});
const modelEnvelope = await readEnvelope(modelResponse, '/api/model-configs/');
if (!modelEnvelope.data?.id || !modelEnvelope.data.is_default) {
  throw new Error('embedding model configuration was not persisted as the default');
}

const listResponse = await fetch(new URL('/api/knowledge-base/', apiURL), {
  headers: authorization,
});
const listEnvelope = await readEnvelope(listResponse, '/api/knowledge-base/');
const knowledgeBases = listEnvelope.data?.list;

if (!Array.isArray(knowledgeBases) || knowledgeBases.length !== 1 || !knowledgeBases[0]?.is_default) {
  throw new Error('expected exactly one default knowledge base for the disposable user');
}

const defaultKnowledgeBase = knowledgeBases[0];
const documentsURL = new URL('/api/document', apiURL);
documentsURL.searchParams.set('kb_id', defaultKnowledgeBase.id);
documentsURL.searchParams.set('page', '1');
documentsURL.searchParams.set('page_size', '20');
const documentsResponse = await fetch(documentsURL, { headers: authorization });
const documentsEnvelope = await readEnvelope(documentsResponse, '/api/document');

if (!Array.isArray(documentsEnvelope.data?.list) || documentsEnvelope.data.total !== 0) {
  throw new Error('expected the disposable default knowledge base to contain no documents');
}

console.log(`Prepared empty default knowledge base and local embedding model for disposable user ${username}.`);
