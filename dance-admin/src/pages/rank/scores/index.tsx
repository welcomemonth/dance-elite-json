import { InboxOutlined } from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';
import {
  PageContainer,
  ProForm,
  ProFormList,
  ProFormSelect,
  ProFormSwitch,
  ProFormText,
  ProTable,
  StepsForm,
} from '@ant-design/pro-components';
import { App, Alert, Card, Col, Row, Space, Typography, Upload } from 'antd';
import React, { useState } from 'react';
import { listEvents } from '@/services/youwumeng/events';
import { AWARD_OPTIONS, boardLabel } from '@/services/youwumeng/labels';
import {
  listSeasons,
  previewScores,
  publishScores,
} from '@/services/youwumeng/seasons';
import type { AgeLevel, DanceType, ScorePreview } from '@/services/youwumeng/typings';

const BOARDS: { level: AgeLevel; dance_type: DanceType }[] = [
  { level: 'U11', dance_type: 'classical' },
  { level: 'U11', dance_type: 'folk' },
  { level: 'U13', dance_type: 'classical' },
  { level: 'U13', dance_type: 'folk' },
  { level: 'U15', dance_type: 'classical' },
  { level: 'U15', dance_type: 'folk' },
];

/** 成绩录入向导（设计文档 8.2）：选择赛事与榜单 → 录入/导入 → 确认发布 */
const ScoreEntry: React.FC = () => {
  const { message } = App.useApp();
  const [seasons, setSeasons] = useState<{ label: string; value: number }[]>([]);
  const [eventOptions, setEventOptions] = useState<{ label: string; value: number }[]>([]);
  const [selection, setSelection] = useState<{ eventId: number; level: AgeLevel; danceType: DanceType } | null>(null);
  const [preview, setPreview] = useState<ScorePreview | null>(null);

  const previewColumns: ProColumns<ScorePreview['rows'][number]>[] = [
    { title: '行号', dataIndex: 'row_no', width: 70 },
    { title: '姓名', dataIndex: 'player_name', width: 120 },
    { title: '证件号', dataIndex: 'id_card_masked', width: 180 },
    {
      title: '匹配',
      dataIndex: 'match_status',
      width: 90,
      render: (_, r) =>
        r.match_status === 'matched' ? <Typography.Text type="success">成功</Typography.Text> : <Typography.Text type="danger">未匹配</Typography.Text>,
    },
    {
      title: '奖项',
      dataIndex: 'award',
      render: (_, r) => AWARD_OPTIONS.find((a) => a.value === r.award)?.label ?? '-',
    },
    { title: '晋级决赛', dataIndex: 'advanced_to_final', render: (_, r) => (r.advanced_to_final ? '是' : '-') },
    { title: '备注', dataIndex: 'remark' },
  ];

  return (
    <PageContainer title="成绩录入">
      <Card>
        <StepsForm
          onFinish={async () => {
            if (!selection || !preview) return false;
            await publishScores(selection.eventId, selection.level, selection.danceType, preview.rows);
            message.success('已发布，小程序端榜单即时更新');
            return true;
          }}
          submitter={{ render: (_, dom) => dom }}
        >
          <StepsForm.StepForm
            title="选择赛事与榜单"
            onFinish={async (values) => {
              const seasonId = values.season_id as number;
              const list = await listEvents({ page: 1, page_size: 100 });
              setEventOptions(list.list.filter((e) => e.season_id === seasonId).map((e) => ({ label: e.name, value: e.event_id })));
              const board = BOARDS[values.board_index as number];
              setSelection({ eventId: values.event_id as number, level: board.level, danceType: board.dance_type });
              return true;
            }}
          >
            <ProFormSelect
              name="season_id"
              label="赛季"
              rules={[{ required: true }]}
              request={async () => {
                const list = await listSeasons();
                setSeasons(list.map((s) => ({ label: s.name, value: s.season_id })));
                return list.map((s) => ({ label: s.name, value: s.season_id }));
              }}
            />
            <ProFormSelect
              name="event_id"
              label="赛事"
              rules={[{ required: true }]}
              options={eventOptions}
              dependencies={['season_id']}
              request={async () => eventOptions}
            />
            <ProFormSelect
              name="board_index"
              label="榜单"
              rules={[{ required: true }]}
              options={BOARDS.map((b, i) => ({ label: boardLabel(b.level, b.dance_type), value: i }))}
            />
            <Alert type="info" showIcon message="同一场赛事 6 个榜单需分别录入；每天只比一个舞种，实际每场只需 3 个榜单。" />
          </StepsForm.StepForm>

          <StepsForm.StepForm
            title="导入 / 录入成绩"
            onFinish={async (values) => {
              if (!selection) return false;
              const rows = (values.rows ?? []) as {
                player_name: string;
                id_card_no?: string;
                award?: string;
                advanced_to_final?: boolean;
                remark?: string;
              }[];
              if (rows.length === 0) {
                message.warning('请至少录入一行成绩');
                return false;
              }
              const result = await previewScores(selection.eventId, selection.level, selection.danceType, { rows });
              setPreview(result);
              return true;
            }}
          >
            <Typography.Title level={5}>在线逐行录入（辅助方式）</Typography.Title>
            <Typography.Paragraph type="secondary">
              主要方式为上传固定格式成绩 Excel（模板列：姓名、证件号、奖项），按证件号唯一匹配选手；
              mock 阶段请使用在线录入，Excel 解析在接入真实后端后启用。
            </Typography.Paragraph>
            <ProFormList
              name="rows"
              copyIconProps={false}
              creatorButtonProps={{ creatorButtonText: '添加一行' }}
            >
              <Space align="baseline" wrap>
                <ProFormText name="player_name" placeholder="选手姓名" rules={[{ required: true }]} />
                <ProFormText name="id_card_no" placeholder="证件号（唯一匹配键）" />
                <ProFormSelect name="award" placeholder="奖项" options={AWARD_OPTIONS} width="sm" />
                <ProFormSwitch name="advanced_to_final" label="晋级决赛" />
                <ProFormText name="remark" placeholder="备注" />
              </Space>
            </ProFormList>
          </StepsForm.StepForm>

          <StepsForm.StepForm title="确认发布">
            {preview && (
              <>
                <Alert
                  type="warning"
                  showIcon
                  style={{ marginBottom: 12 }}
                  title={`共 ${preview.total_rows} 行：匹配成功 ${preview.matched}，未匹配 ${preview.unmatched}。未匹配行发布前请修正证件号或手动建档。`}
                />
                <ProTable<ScorePreview['rows'][number]>
                  rowKey="row_no"
                  size="small"
                  search={false}
                  options={false}
                  pagination={false}
                  dataSource={preview.rows}
                  columns={previewColumns}
                />
                <Alert
                  type="info"
                  showIcon
                  style={{ marginTop: 12 }}
                  title="发布后该榜单小程序端即时更新；已发布成绩的修改须走「积分与榜单 → 成绩纠错」流程，全程留痕。"
                />
              </>
            )}
          </StepsForm.StepForm>
        </StepsForm>
      </Card>
    </PageContainer>
  );
};

export default ScoreEntry;
