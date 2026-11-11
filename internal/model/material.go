package model

// 参赛资料 8 项字段名（API 文档 3.3/4.1 的 material_status.missing 口径）。
const (
	MaterialFieldPhoto          = "photo"
	MaterialFieldAudio          = "audio"
	MaterialFieldProgramName    = "program_name"
	MaterialFieldTeacherName    = "teacher_name"
	MaterialFieldContactPhone   = "contact_phone"
	MaterialFieldExpressAddress = "express_address"
	MaterialFieldWorkIntro      = "work_intro"
	MaterialFieldSpecialReq     = "special_req"
)

// MaterialTotalFields 是资料完成度的总项数（8 项，API 文档 3.3）。
const MaterialTotalFields = 8

// MaterialStatus 是资料完成度（API 文档 3.3/4.1）：
// 由字段是否为空实时计算，不存状态；Missing 为空即资料已齐。
type MaterialStatus struct {
	Total     int      `json:"total"`
	Completed int      `json:"completed"`
	Missing   []string `json:"missing"`
}

// MaterialsSnapshot 是报名单当前资料快照（API 文档 3.4/4.1）。
// 文件字段输出对外访问 URL（/static/...），文本字段未传为 null。
type MaterialsSnapshot struct {
	PhotoURL       *string `json:"photo_url"`
	AudioURL       *string `json:"audio_url"`
	AudioFilename  *string `json:"audio_filename"`
	ProgramName    *string `json:"program_name"`
	TeacherName    *string `json:"teacher_name"`
	ContactPhone   *string `json:"contact_phone"`
	ExpressAddress *string `json:"express_address"`
	WorkIntro      *string `json:"work_intro"`
	SpecialReq     *string `json:"special_req"`
	UpdatedAt      *string `json:"updated_at"`
}

// UpdateMaterialsResponse 是更新参赛资料（API 文档 4.1）的响应。
type UpdateMaterialsResponse struct {
	MaterialStatus MaterialStatus    `json:"material_status"`
	Materials      MaterialsSnapshot `json:"materials"`
}
