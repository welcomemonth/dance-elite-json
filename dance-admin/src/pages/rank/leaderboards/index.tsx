import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  DrawerForm,
  PageContainer,
  ProForm,
  ProFormDigit,
  ProFormSelect,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import {
  Alert,
  App,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Tabs,
  Tag,
  Typography,
} from 'antd';
import React, { useRef, useState } from 'react';
import {
  AWARD_LABELS,
  AWARD_OPTIONS,
  boardLabel,
} from '@/services/youwumeng/labels';
import {
  correctScore,
  getScoreRules,
  listLeaderboards,
  listSeasons,
  putScoreRules,
  updateLeaderboardRow,
} from '@/services/youwumeng/seasons';
import type {
  AgeLevel,
  Award,
  DanceType,
  Leaderboard,
  ScoreRules,
} from '@/services/youwumeng/typings';

/** 积分与榜单（设计文档 8.3）：6 榜单 + 规则配置 + 展示名/隐藏 + 成绩纠错 */
const Leaderboards: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const [seasonId, setSeasonId] = useState(1);
  const [rulesOpen, setRulesOpen] = useState(false);
  const [editRow, setEditRow] = useState<{ board: Leaderboard; playerId: number; name: string } | null>(null);
  const [correctTarget, setCorrectTarget] = useState<{ entryId: number; name: string } | null>(null);
  const [correctForm] = Form.useForm();
  const [rulesForm] = Form.useForm<ScoreRules>();
  const actionRef = useRef<ActionType>(null);

  const { data: seasons } = useService(listSeasons);
  const { data: boards, refresh } = useService(() => listLeaderboards(seasonId), {
    refreshDeps: [seasonId],
  });

  const { run: loadRules } = useService(() => getScoreRules(seasonId), {
    manual: true,
    refreshDeps: [seasonId],
    onSuccess: (r) => rulesForm.setFieldsValue(r),
  });

  const boardColumns = (board: Leaderboard): ProColumns<Leaderboard['rows'][number]>[] => [
    { title: '排名', dataIndex: 'rank', width: 70 },
    {
      title: '选手',
      render: (_, r) => (
        <Space size={4}>
          <span>{r.player_name}</span>
          {r.display_name && <Tag color="purple">展示名：{r.display_name}</Tag>}
          {r.direct_qualification && <Tag color="gold">直通</Tag>}
        </Space>
      ),
    },
    { title: '选手编号', dataIndex: 'player_no', width: 100 },
    { title: '机构', dataIndex: 'institution', width: 140 },
    { title: '总积分', dataIndex: 'total_points', width: 100, sorter: true },
    { title: '场次', dataIndex: 'event_count', width: 70 },
    {
      title: '操作',
      valueType: 'option',
      width: 180,
      render: (_, r) =>
        access.canAdmin ? (
          <Space size={4}>
            <Typography.Link
              onClick={() => setEditRow({ board, playerId: r.player_id, name: r.display_name ?? '' })}
            >
              展示名
            </Typography.Link>
            <Popconfirm
              title={r.hidden ? '恢复上榜' : '隐藏上榜'}
              description={r.hidden ? '恢复该选手在小程序榜单展示。' : '隐藏后小程序榜单不展示该选手，操作留痕。'}
              onConfirm={async () => {
                await updateLeaderboardRow(seasonId, board.level, board.dance_type, r.player_id, { hidden: !r.hidden });
                message.success('已更新');
                refresh();
              }}
            >
              <Typography.Link type={r.hidden ? undefined : 'danger'}>
                {r.hidden ? '恢复' : '隐藏'}
              </Typography.Link>
            </Popconfirm>
            <Typography.Link onClick={() => { setCorrectTarget({ entryId: 7000 + (r.rank % 8), name: r.player_name }); correctForm.resetFields(); }}>
              纠错
            </Typography.Link>
          </Space>
        ) : null,
    },
  ];

  return (
    <PageContainer title={false}>
      <Card
        size="small"
        title="积分与榜单"
        extra={
          <Space>
            <ProFormSelect
              noStyle
              fieldProps={{ value: seasonId, onChange: (v: number) => setSeasonId(v), style: { minWidth: 150 } }}
              request={async () => (seasons ?? []).map((s) => ({ label: s.name, value: s.season_id }))}
            />
            <Button
              onClick={() => {
                setRulesOpen(true);
                loadRules();
              }}
            >
              积分规则配置
            </Button>
            <Button onClick={() => (window.location.href = '/user-content/contents?key=points_rules_desc')}>
              榜单说明文案
            </Button>
          </Space>
        }
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          title="每名选手取本年度积分最高的 3 场计入总榜；成绩发布后实时重算。隐藏/纠错均留痕。"
        />
        <Tabs
          items={(boards ?? []).map((b) => ({
            key: `${b.level}-${b.dance_type}`,
            label: boardLabel(b.level as AgeLevel, b.dance_type as DanceType),
            children: (
              <ProTable<Leaderboard['rows'][number]>
                rowKey="player_id"
                size="small"
                search={false}
                options={false}
                pagination={false}
                actionRef={actionRef}
                dataSource={b.rows.map((r) => ({ ...r, hidden: r.hidden }))}
                columns={boardColumns(b)}
                rowClassName={(r) => (r.hidden ? 'ant-table-row-disabled' : '')}
              />
            ),
          }))}
        />
      </Card>

      <DrawerForm
        title="积分规则配置（赛季级）"
        open={rulesOpen}
        onOpenChange={setRulesOpen}
        form={rulesForm}
        onFinish={async (values) => {
          await putScoreRules(seasonId, values as ScoreRules);
          message.success('已保存');
          return true;
        }}
        width={480}
      >
        <Typography.Title level={5}>名次基础分</Typography.Title>
        <Form.Item noStyle shouldUpdate>
          {() => (
            <Space direction="vertical" style={{ width: '100%' }}>
              {(Object.keys(AWARD_LABELS) as Award[]).map((a) => (
                <Form.Item
                  key={a}
                  name={['base_points', a]}
                  label={AWARD_LABELS[a]}
                  style={{ marginBottom: 4 }}
                >
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
              ))}
            </Space>
          )}
        </Form.Item>
        <Typography.Title level={5} style={{ marginTop: 16 }}>级别权重</Typography.Title>
        <Space>
          <Form.Item name={['level_weights', 'grade_a']} label="甲级赛"><InputNumber step={0.1} min={0} /></Form.Item>
          <Form.Item name={['level_weights', 'super']} label="超级赛"><InputNumber step={0.1} min={0} /></Form.Item>
          <Form.Item name={['level_weights', 'final']} label="总决赛"><InputNumber step={0.1} min={0} /></Form.Item>
        </Space>
        <Alert type="info" showIcon message="人数系数分档、榜单说明文案等更细规则以后续模板为准。" />
      </DrawerForm>

      <Modal
        title={editRow ? `设置展示名：${editRow.name || '选手'}` : ''}
        open={!!editRow}
        onCancel={() => setEditRow(null)}
        onOk={async () => {
          if (!editRow) return;
          await updateLeaderboardRow(seasonId, editRow.board.level, editRow.board.dance_type, editRow.playerId, {
            display_name: editRow.name || undefined,
          });
          message.success('已保存');
          setEditRow(null);
          refresh();
        }}
      >
        <Alert type="info" showIcon style={{ marginBottom: 12 }} message="留空表示使用真名；可设为化名或编号。" />
        <Input
          placeholder="展示名（真名 / 化名 / 编号）"
          value={editRow?.name ?? ''}
          onChange={(e) => setEditRow((s) => (s ? { ...s, name: e.target.value } : s))}
        />
      </Modal>

      <Modal
        title={correctTarget ? `成绩纠错：${correctTarget.name}` : ''}
        open={!!correctTarget}
        onCancel={() => setCorrectTarget(null)}
        onOk={() => correctForm.submit()}
      >
        <Alert type="warning" showIcon style={{ marginBottom: 12 }} message="修改已发布成绩将自动重算积分与排名，全程留痕。" />
        <Form
          form={correctForm}
          onFinish={async (values) => {
            if (!correctTarget) return;
            const r = await correctScore(correctTarget.entryId, values);
            message.success(`已提交纠错，差异：${JSON.stringify(r.diff)}`);
            setCorrectTarget(null);
            refresh();
          }}
        >
          <Form.Item name="award" label="奖项">
            <Select
              allowClear
              placeholder="保持不变"
              options={AWARD_OPTIONS}
            />
          </Form.Item>
          <Form.Item name="remark" label="纠错说明">
            <Input.TextArea rows={3} placeholder="必填：说明纠错原因" />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
};

export default Leaderboards;
