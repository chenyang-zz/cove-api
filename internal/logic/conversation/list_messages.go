package conversation

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/repository"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

// ListMessagesLogic contains the listMessages use case.
type ListMessagesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

// NewListMessagesLogic creates a ListMessagesLogic.
func NewListMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMessagesLogic {
	return &ListMessagesLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.conversation.listmessages"),
	}
}

// ListMessages 获取会话消息列表（before 游标分页，支持向上滚动加载历史）。
func (l *ListMessagesLogic) ListMessages(userID uuid.UUID, input *request.ListMessagesRequest) (*response.MessageListResponse, error) {
	if input == nil {
		return nil, xerr.BadRequest("会话消息列表参数不能为空")
	}

	conversationID, err := conversationIDFromInput(&input.UriConversationIDRequest)
	if err != nil {
		return nil, err
	}

	// 显式校验会话归属，与 rename/delete 一致返回 404
	if _, err := l.svcCtx.ConversationRepo.FindByID(l.ctx, userID, conversationID); err != nil {
		return nil, err
	}

	beforeID, err := parseOptionalBeforeID(input.Before)
	if err != nil {
		return nil, err
	}

	// 按会话分页拉取消息，默认最近一页
	messages, hasMore, err := l.svcCtx.MessageRepo.ListPage(l.ctx, userID, repository.MessageListQuery{
		ConversationID: conversationID,
		BeforeID:       beforeID,
		Limit:          normalizeMessageLimit(input.Limit),
	})
	if err != nil {
		return nil, err
	}

	// 拉取会话反馈后只保留本页消息的评分
	feedbacks, err := l.svcCtx.MessageFeedbackRepo.ListByConversationID(l.ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	pageMsgIDs := make(map[uuid.UUID]struct{}, len(messages))
	for _, message := range messages {
		pageMsgIDs[message.ID] = struct{}{}
	}
	rateByMsgID := make(map[uuid.UUID]string, len(feedbacks))
	for _, feedback := range feedbacks {
		if _, ok := pageMsgIDs[feedback.MessageID]; !ok {
			continue
		}
		rateByMsgID[feedback.MessageID] = feedback.Rating
	}

	// user 消息里存的图片 key 转成可访问 url（历史还原图片显示）
	imagesMap := make(map[uuid.UUID][]string, len(messages))
	needSignerURL := l.svcCtx.URLSigner != nil
	for _, message := range messages {
		if message.MetaData == nil || len(message.MetaData.ImageKeys) == 0 {
			continue
		}
		images := make([]string, 0, len(message.MetaData.ImageKeys))
		if needSignerURL {
			for _, imageKey := range message.MetaData.ImageKeys {
				images = append(images, l.svcCtx.URLSigner.URL(imageKey))
			}
		} else {
			images = append(images, message.MetaData.ImageKeys...)
		}
		imagesMap[message.ID] = images
	}

	return mapper.MessagesToListResponse(messages, imagesMap, rateByMsgID, hasMore), nil
}
