package auth

import (
	"context"
	"log/slog"

	"github.com/boxify/api-go/internal/infrastructure/storage"
	"github.com/boxify/api-go/internal/mapper"
	"github.com/boxify/api-go/internal/observability/xlog"
	"github.com/boxify/api-go/internal/svc"
	"github.com/boxify/api-go/internal/transport/http/request"
	"github.com/boxify/api-go/internal/transport/http/response"
	"github.com/boxify/api-go/internal/util/uploadfile"
	"github.com/boxify/api-go/internal/xerr"
	"github.com/google/uuid"
)

var SupportAvatarExts = []string{".jpg", ".jpeg", ".png", ".webp", ".gif"}

const MaxAvatarSize = 5 * 1024 * 1024 // 5MB

type UpdateAvatarLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	log    *slog.Logger
}

func NewUpdateAvatarLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAvatarLogic {
	return &UpdateAvatarLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		log:    xlog.Component("logic.auth.updateavatar"),
	}
}

func (l *UpdateAvatarLogic) UpdateAvatar(userID uuid.UUID, input *request.FileRequest) (*response.UserResponse, error) {
	if input == nil || input.File == nil {
		return nil, xerr.BadRequest("上传文件不能为空")
	}
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, userID)
	if err != nil {
		return nil, err
	}

	fileInfo, err := uploadfile.Read(input.File, MaxAvatarSize, "头像大小不能超过 5MB 限制", "读取头像文件出错")
	if err != nil {
		return nil, err
	}
	ext := fileInfo.Ext
	support := false
	for _, supportExt := range SupportAvatarExts {
		if supportExt == ext {
			support = true
			break
		}
	}
	if !support {
		return nil, xerr.BadRequestf("不支持的图谱类型: %s", ext)
	}

	fileKey := storage.BuildFileKey(userID, "avatar", uuid.New(), ext)
	err = l.svcCtx.Storage.Put(l.ctx, fileKey, fileInfo.Content)
	if err != nil {
		return nil, err
	}

	user.Avatar = &fileKey
	user, err = l.svcCtx.UserRepo.Update(l.ctx, user)
	if err != nil {
		return nil, err
	}

	return mapper.UserToResponse(user), nil
}
