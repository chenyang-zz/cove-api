/**
 * @Time   : 2026/6/26 20:56
 * @Author : chenyangzhao542@gmail.com
 * @File   : model.go
 **/

package domain

type ModelProviderType string

const (
	OpenaiProvider ModelProviderType = "openai"
)

type ModelType string

const (
	ChatModelType      ModelType = "chat"
	Multimodal         ModelType = "multimodal"
	EmbeddingModelType ModelType = "embedding"
	RerankModelType    ModelType = "rerank"
	WebsearchModelType ModelType = "websearch"
	AsrModelType       ModelType = "asr"
)
