package chat

import (
	"context"
	"maps"
	"strings"

	"log/slog"

	"github.com/boxify/api-go/internal/core/llm"
	flow "github.com/boxify/api-go/internal/domain/flow"
	flowchat "github.com/boxify/api-go/internal/domain/flow/chat"
	"github.com/boxify/api-go/internal/domain/types"
	"github.com/boxify/api-go/internal/infrastructure/realtime"
	"github.com/boxify/api-go/internal/models"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

const defaultChatTemperature = 0.7

// ChatStreamLogic contains the chatStream use case.
type ChatStreamLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewChatStreamLogic creates a ChatStreamLogic.
func NewChatStreamLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ChatStreamLogic {
	return &ChatStreamLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.chat.chatstream"),
	}
}

// ChatStream 流式聊天
func (l *ChatStreamLogic) ChatStream(userID uuid.UUID, input *request.ChatStreamRequest) (<-chan types.Event, error) {
	// 生成动作不在当前协程中，而是一个独立 协程 的后台任务生成
	// 通过 Redis 频道广播 token；本协程只「订阅频道并转发」给当前客户端
	// 这样客户端中途断开（切页面/关标签）只会停止转发，后台生成照常跑完并落库——回来重拉历史能看到完整回复
	// 生成中重连还能续传（见 resume_events）
	if l.svcCtx == nil || l.svcCtx.Realtime == nil {
		return nil, xerr.Internal("实时消息服务未初始化", nil)
	}

	userText := strings.TrimSpace(input.Message)

	attachments := make([]*types.MessageAttachment, 0, len(input.Attachments))
	for _, attachment := range input.Attachments {
		if attachment.Text != "" {
			attachments = append(attachments, &types.MessageAttachment{
				Content:  attachment.Text,
				FileName: attachment.FileName,
			})
		}
	}

	conversation, err := l.ensureConversation(userID, input.ConversationID, input.Message)
	if err != nil {
		return nil, err
	}

	// AI 主动开场白（今日回顾「聊聊」）：仅新会话首轮，先把开场白作为
	// assistant 消息落库，使其进入对话历史，模型回复时能接住这个话题
	greeting := strings.TrimSpace(input.Greeting)
	messageCount, err := l.svcCtx.MessageRepo.Count(l.ctx, conversation.ID)
	if err != nil {
		return nil, err
	}
	if len(greeting) != 0 && messageCount == 0 {
		l.log.InfoContext(l.ctx, "添加开场白", slog.String("greeting", greeting))
		_, err = l.svcCtx.MessageRepo.Create(l.ctx, userID, &models.Message{
			ConversationID: conversation.ID,
			Role:           string(llm.AssistantRole),
			Content:        greeting,
		})
		if err != nil {
			l.log.WarnContext(l.ctx, "添加开场白失败", slog.String("greeting", greeting), slog.String("error", err.Error()))
		}
	}
	userMessage, err := l.svcCtx.MessageRepo.Create(l.ctx, userID, &models.Message{
		ConversationID: conversation.ID,
		Role:           string(llm.UserRole),
		Content:        userText,
		MetaData:       &models.MessageMetaData{ImageKeys: append([]string(nil), input.ImageKeys...)},
	})
	if err != nil {
		return nil, err
	}

	topic := realtime.ConversationTopic(conversation.ID)
	subscription, err := l.svcCtx.Realtime.Subscribe(l.ctx, topic)
	if err != nil {
		return nil, err
	}

	// 创建事件通道并启动后台 goroutine 转发事件
	events := make(chan types.Event, 16)
	go func() {
		select {
		case <-l.ctx.Done():
			close(events)
			_ = subscription.Close(context.Background())
			return
		case events <- types.NewMetaEvent(conversation.ID, conversation.Title):
		}
		err = realtime.Forward(l.ctx, subscription, events, realtime.ForwardOptions{})
		if err != nil {
			l.log.WarnContext(l.ctx, "转发事件失败", slog.String("error", err.Error()))
		}
	}()

	// 启动后台 goroutine 生成回复
	go func() {
		// TODO: 后续补 Redis 回合锁和断线续传缓冲，避免同会话并发生成。
		l.runChatTurnBG(context.WithoutCancel(l.ctx), userID, conversation.ID, userMessage.ID, input, attachments)
	}()
	return events, nil
}

// runChatTurnBG 后台生成回复
func (l *ChatStreamLogic) runChatTurnBG(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
	userMessageID uuid.UUID,
	input *request.ChatStreamRequest,
	attachments []*types.MessageAttachment,
) {
	topic := realtime.ConversationTopic(conversationID)

	// 检查会话存在
	if _, err := l.svcCtx.ConversationRepo.FindByID(ctx, userID, conversationID); err != nil {
		_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewErrorEvent("会话不存在"))
		return
	}
	agentConfig, err := l.chatAgentConfig(ctx, userID)
	if err != nil {
		l.log.WarnContext(ctx, "获取聊天 Agent 配置失败", slog.String("error", err.Error()))
		_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewErrorEvent("生成失败："+err.Error()))
		return
	}
	persona, err := l.chatActivePersona(ctx, userID)
	if err != nil {
		l.log.WarnContext(ctx, "获取生效角色失败", slog.String("error", err.Error()))
		_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewErrorEvent("生成失败："+err.Error()))
		return
	}
	runtimeConfig := resolveChatRuntimeConfig(input, agentConfig, persona)
	l.log.InfoContext(ctx, "聊天最终系统提示词",
		slog.String("user_id", userID.String()),
		slog.String("conversation_id", conversationID.String()),
		slog.String("system_prompt", runtimeConfig.SystemPrompt),
	)

	messageCh, err := flowchat.NewOrchestrator(l.svcCtx).Run(ctx, flowchat.Input{
		UserID:               userID,
		ConversationID:       conversationID,
		CurrentUserMessageID: userMessageID,
		Message:              input.Message,
		Attachments:          attachments,
		EnableKnowledge:      runtimeConfig.EnableKnowledge,
		Temperature:          runtimeConfig.Temperature,
		SystemPrompt:         runtimeConfig.SystemPrompt,
	})
	if err != nil {
		l.log.WarnContext(ctx, "后台生成回复失败", slog.String("error", err.Error()))
		_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewErrorEvent("生成失败："+err.Error()))
		return
	}

	var assistantMessageID string
	parts := make([]models.MessagePart, 0, 8)
	sentToken := false
	for message := range messageCh {
		switch msg := message.(type) {
		case *flow.AssistantMessage:
			answer := strings.TrimSpace(msg.Answer)
			// 用最终 answer 对齐 parts 正文，content 仍以 answer 为真源
			parts = finalizePartsWithAnswer(parts, answer)
			assistantMsg, saveErr := l.svcCtx.MessageRepo.Create(ctx, userID, &models.Message{
				ConversationID: conversationID,
				Role:           string(llm.AssistantRole),
				Content:        answer,
				MetaData:       buildAssistantMeta(parts, false),
			})
			if saveErr != nil {
				l.log.WarnContext(ctx, "保存AI回复失败", slog.String("error", saveErr.Error()))
				_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewErrorEvent("保存回复失败："+saveErr.Error()))
				return
			}
			assistantMessageID = assistantMsg.ID.String()
			// 如果没有发送过 token，则在这里发送最终 answer 作为 token 事件，确保客户端能收到完整回复
			if answer != "" && !sentToken {
				_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewTokenEvent(answer))
			}
		case *flow.ErrorMessage:
			partial := strings.TrimSpace(msg.Partial)
			if partial != "" {
				parts = finalizePartsWithAnswer(parts, partial)
				if _, saveErr := l.svcCtx.MessageRepo.Create(ctx, userID, &models.Message{
					ConversationID: conversationID,
					Role:           string(llm.AssistantRole),
					Content:        partial,
					MetaData:       buildAssistantMeta(parts, true),
				}); saveErr != nil {
					l.log.WarnContext(ctx, "保存部分回复失败", slog.String("error", saveErr.Error()))
				}
			} else if len(parts) > 0 {
				// 无 partial 文本但已有片段时仍落库，便于历史还原中断前状态
				if _, saveErr := l.svcCtx.MessageRepo.Create(ctx, userID, &models.Message{
					ConversationID: conversationID,
					Role:           string(llm.AssistantRole),
					Content:        "",
					MetaData:       buildAssistantMeta(parts, true),
				}); saveErr != nil {
					l.log.WarnContext(ctx, "保存中断元数据失败", slog.String("error", saveErr.Error()))
				}
			}
			messageText := strings.TrimSpace(msg.Message)
			if messageText == "" && msg.Err != nil {
				messageText = msg.Err.Error()
			}
			l.log.WarnContext(ctx, "后台生成回复失败", slog.String("error", messageText))
			_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewErrorEvent("生成失败："+messageText))
			return
		case *flow.ToolCallMessage:
			if msg != nil {
				parts = appendToolCallPart(parts, msg.Tool, msg.Input, msg.Iteration, msg.ToolCallID)
				_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewToolCallEvent(msg.Tool, cloneFlowInput(msg.Input), msg.Iteration, msg.ToolCallID))
			}
		case *flow.ToolResultMessage:
			if msg != nil {
				obs := truncateObservation(msg.Observation)
				parts = appendToolResultPart(parts, msg.Tool, msg.Input, obs, msg.Error, msg.Iteration, msg.ToolCallID)
				_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewToolResultEvent(msg.Tool, cloneFlowInput(msg.Input), msg.Observation, msg.Error, msg.Iteration, msg.ToolCallID))
			}
		case *flow.PartialMessage:
			if msg != nil && msg.Text != "" {
				// Partial 入 parts（相邻 text merge），供历史还原交错正文
				parts = appendTextPart(parts, msg.Text)
				sentToken = true
				_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewTokenEvent(msg.Text))
			}
		case *flow.ThinkMessage:
			if msg != nil {
				// think 为瞬时 UI 状态，不写入 message parts
				_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewThinkEvent(msg.Status, msg.Iteration))
			}
		case *flow.DoneMessage:
			_ = l.svcCtx.Realtime.Publish(ctx, topic, types.NewDoneEvent(assistantMessageID))
		default:
			l.log.WarnContext(ctx, "忽略未知 Flow 消息")
		}
	}
	// TODO: 后续接入记忆萃取、图片入库、情绪分析等副作用，失败不能影响主回复。
}

func cloneFlowInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	maps.Copy(out, input)
	return out
}

type chatRuntimeConfig struct {
	EnableKnowledge bool
	Temperature     float64
	SystemPrompt    string
}

// chatAgentConfig 获取Agent配置
func (l *ChatStreamLogic) chatAgentConfig(ctx context.Context, userID uuid.UUID) (*models.AgentConfig, error) {
	if l.svcCtx == nil || l.svcCtx.AgentConfigRepo == nil {
		return nil, nil
	}
	config, err := l.svcCtx.AgentConfigRepo.FindByUserID(ctx, userID)
	if err != nil {
		if xerr.From(err).Kind == xerr.KindNotFound {
			return nil, nil
		}
		return nil, err
	}
	return config, nil
}

// chatActivePersona 获取用户当前生效的 AgentPersona；无生效角色时返回 nil。
func (l *ChatStreamLogic) chatActivePersona(ctx context.Context, userID uuid.UUID) (*models.AgentPersona, error) {
	if l.svcCtx == nil || l.svcCtx.AgentPersonaRepo == nil {
		return nil, nil
	}
	return l.svcCtx.AgentPersonaRepo.FindActive(ctx, userID)
}

// resolveChatRuntimeConfig 归一化聊天运行参数，并在 logic 层提供业务默认值兜底。
// 有人格 soul/identity 时注入 # Soul / # Identity；否则注入默认 Cove 身份。
func resolveChatRuntimeConfig(input *request.ChatStreamRequest, agentConfig *models.AgentConfig, persona *models.AgentPersona) chatRuntimeConfig {
	config := chatRuntimeConfig{
		EnableKnowledge: false,
		Temperature:     defaultChatTemperature,
	}
	if agentConfig != nil && agentConfig.Temperature > 0 {
		config.Temperature = agentConfig.Temperature
	}
	agentPrompt := ""
	if agentConfig != nil {
		config.EnableKnowledge = agentConfig.EnableKnowledge
		agentPrompt = strings.TrimSpace(agentConfig.SystemPrompt)
	}
	soul, identity := "", ""
	if persona != nil {
		soul = persona.Soul
		identity = persona.Identity
	}
	config.SystemPrompt = buildChatSystemPrompt(soul, identity, agentPrompt)
	if input != nil && input.EnableKnowledge != nil {
		config.EnableKnowledge = *input.EnableKnowledge
	}
	return config
}

// ensureConversation 确保会话存在
func (l *ChatStreamLogic) ensureConversation(userID uuid.UUID, conversationIDStr string, message string) (*models.Conversation, error) {
	conversationID, err := parseConversationID(conversationIDStr)
	if err == nil {
		conversation, err := l.svcCtx.ConversationRepo.FindByID(l.ctx, userID, conversationID)
		if err == nil {
			return conversation, nil
		}
	}

	var title string
	if message == "" {
		title = "新对话"
	} else if len(message) <= 20 {
		title = message
	} else {
		title = message[:20]
	}

	return l.svcCtx.ConversationRepo.Create(l.ctx, userID, &models.Conversation{Title: title})
}
