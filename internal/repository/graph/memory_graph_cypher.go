/**
 * @Time   : 2026/6/23 23:09
 * @Author : chenyangzhao542@gmail.com
 * @File   : memory_graph_cypher.go
 **/

package graph

const listEntitiesByTypeCypher = `
MATCH (e:Entity {user_id: $user_id, type: $type})
RETURN e {
	.id,
	.user_id,
	.name,
	.type,
	.description,
	.aliases,
	.name_embedding,
	.community_id,
	.importance,
	.confidence,
	.memory_layer,
	.access_count,
	.last_access_at,
	.mention_count,
	.connect_strength,
	.core_facts,
	.traits,
	.last_consolidated_at,
	.created_at
} AS entity
`

const saveDialoguesCypher = `
UNWIND $rows AS row
MERGE (n:Dialogue {id: row.id})
SET n.user_id = row.user_id,
	n.content = row.content,
	n.source = row.source,
	n.source_message_id = row.source_message_id,
	n.dialog_at = row.dialog_at,
	n.created_at = row.created_at
RETURN count(n) AS cnt
`

const saveChunksCypher = `
UNWIND $rows AS row
MERGE (n:Chunk {id: row.id})
SET n.user_id = row.user_id,
	n.dialog_id = row.dialog_id,
	n.content = row.content,
	n.speaker = row.speaker,
	n.sequence = row.sequence,
	n.created_at = row.created_at
WITH n, row
MATCH (d:Dialogue {id: row.dialogue_id})
MERGE (d)-[:HAS_CHUNK]->(n)
RETURN count(n) AS cnt
`

const saveStatementsCypher = `
UNWIND $rows AS row
MERGE (n:Statement {id: row.id})
SET n.user_id = row.user_id,
	n.chunk_id = row.chunk_id,
	n.statement = row.statement,
	n.stmt_type = row.stmt_type,
	n.temporal_type = row.temporal_type,
	n.speaker = row.speaker,
	n.valid_at = row.valid_at,
	n.invalid_at = row.invalid_at,
	n.dialog_at = row.dialog_at,
	n.embedding = row.embedding,
	n.importance = row.importance,
	n.confidence = row.confidence,
	n.memory_layer = coalesce(n.memory_layer, row.memory_layer),
	n.access_count = coalesce(n.access_count, row.access_count),
	n.has_emotional_state = row.has_emotional_state,
	n.emotion_type = row.emotion_type,
	n.emotion_intensity = row.emotion_intensity,
	n.emotion_keywords = row.emotion_keywords,
	n.created_at = row.created_at
WITH n, row
MATCH (c:Chunk {id:row.chunk_id})
MERGE (c)-[:HAS_STATEMENT]->(n)
RETURN count(n) AS cnt
`

// 实体: MERGE by id；description/aliases 增量合并由服务层在写入前算好
// 动力学属性: importance 取较大值、mention_count 累加、layer/access 保留已有
const saveEntitiesCypher = `
UNWIND $rows as row
MERGE (n:Entity {id: row.id})
SET n.user_id = row.user_id,
	n.name = row.name,
	n.type = row.type,
	n.description = row.description,
	n.aliases = row.aliases,
	n.name_embedding = row.name_embedding,
	n.community_id = row.community_id,
	n.importance = CASE
		WHEN n.importance IS NULL THEN row.importance
		ELSE CASE WHEN row.importance > n.importance THEN row.importance ELSE n.importance END
	END,
	n.confidence = row.confidence,
	n.memory_layer = coalesce(n.memory_layer, row.memory_layer)
	n.access_count = coalesce(n.access_count, row.access_count),
	n.mention_count = coalesce(n.mention_count, row.mention_count),
	n.connect_strength = CASE
		WHEN n.connect_strength IS NULL OR n.content_strength = '' THEN row.connect_strength
		WHEN n.connect_strength = row.connect_strength THEN n.connect_strength
		ELSE 'both'
	END,
	n.core_facts = coalesce(n.core_facts, row.core_facts),
	n.traits = coalesce(n.traits, row.traits),
	n.created_at = coalesce(n.created_at, row.created_at)
RETURN count(n) AS cnt
`

const saveEventsCypher = `
UNWIND $rows AS row
MERGE (n:Event {id: row.id})
SET n.user_id = row.user_id,
	n.title = row.title,
	n.description = row.description,
	n.event_time = row.event_time,
	n.embedding = row.embedding,
	n.created_at = row.created_at
RETURN count(n) as cnt
`

const saveMentionsCypher = `
UNWIND $rows AS row
MATCH (s:Statement {id: row.statement_id})
MATCH (e:Entity {id: row.entity_id})
MERGE (s)-[r:MENTIONS]->(e)
SET r.user_id = row.user_id,
	r.connect_strength = row.connect_strength,
	r.created_at = row.created_at
RETURN count(r) AS cnt
`

const saveRelationsCypher = `
UNWIND $rows AS row
MATCH (a:Entity {id: row.source_id})
MATCH (b:Entity {id: row.target_id})
MERGE (a)-[r:RELATION {predicate: row.predicate, target_id: row.target_id}]->(b)
SET r.id = row.id,
	r.user_id = row.user_id,
	r.predicate_surface = row.predicate_surface,
	r.source_text = row.source_text,
	r.statement_id = row.statement_id,
	r.value = row.value,
	r.valid_at = row.valid_at,
	r.invalid_at = row.invalid_at,
	r.importance = CASE
		WHEN r.importance IS NULL THEN row.importance
		ELSE CASE WHEN row.importance > r.importance THEN row.importance ELSE r.importance END
	END,
	r.confidence = row.confidence,
	r.access_count = coalesce(r.access_count, row.access_count),
	r.created_at = row.created_at
RETURN count(r) AS cnt
`

const saveInvolvesCypher = `
UNWIND $rows AS row
MATCH (ev:Event {id: row.event_id})
MATCH (e:Entity {id: row.entity_id})
MERGE (ev)-[r:INVOLVES]->(e)
SET r.user_id = row.user_id,
	r.role = row.role,
	r.created_at = row.created_at
RETURN count(r) AS cnt
`
