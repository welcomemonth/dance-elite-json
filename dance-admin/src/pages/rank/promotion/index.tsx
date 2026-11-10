import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ModalForm,
  PageContainer,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import { Alert, App, Button, Card, Popconfirm, Space, Tabs, Tag, Typography } from 'antd';
import React, { useRef, useState } from 'react';
import { formatDateTime } from '@/services/youwumeng/format';
import {
  adjustFinalsRoster,
  getFinalsRoster,
  grantQualificationCard,
  listQualificationCards,
  listSeasons,
  revokeQualificationCard,
} from '@/services/youwumeng/seasons';

/** 晋级管理（设计文档 8.4）：直通卡 + 总决赛资格名单 */
const Promotion: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const [seasonId, setSeasonId] = useState(1);
  const cardRef = useRef<ActionType>(null);
  const rosterRef = useRef<ActionType>(null);

  const { data: seasons } = useService(listSeasons);
  const { data: cards, refresh: refreshCards } = useService(
    () => listQualificationCards(seasonId),
    { refreshDeps: [seasonId] },
  );
  const { data: roster, refresh: refreshRoster } = useService(
    () => getFinalsRoster(seasonId),
    { refreshDeps: [seasonId] },
  );

  const cardColumns: ProColumns<NonNullable<typeof cards>[number]>[] = [
    { title: '选手', dataIndex: 'player_name', width: 140 },
    { title: '选手编号', dataIndex: 'player_no', width: 100 },
    { title: '榜单', dataIndex: 'board', width: 140 },
    {
      title: '来源',
      dataIndex: 'source',
      width: 90,
      render: (_, r) =>
        r.source === 'auto' ? <Tag color="blue">自动</Tag> : <Tag color="purple">手动增补</Tag>,
    },
    { title: '原因', dataIndex: 'reason', render: (_, r) => r.reason ?? '-' },
    { title: '授予时间', dataIndex: 'granted_at', width: 150, render: (_, r) => formatDateTime(r.granted_at) },
    {
      title: '操作',
      valueType: 'option',
      width: 120,
      render: (_, r) =>
        access.canAdmin ? (
          <Popconfirm
            title="取消直通卡"
            description="取消后该选手失去直通资格，操作留痕。"
            onConfirm={async () => {
              await revokeQualificationCard(seasonId, r.card_id);
              message.success('已取消');
              refreshCards();
              refreshRoster();
            }}
          >
            <Typography.Link type="danger">取消</Typography.Link>
          </Popconfirm>
        ) : null,
    },
  ];

  const rosterColumns: ProColumns<NonNullable<typeof roster>[number]>[] = [
    { title: '排名', dataIndex: 'rank', width: 70 },
    { title: '选手', dataIndex: 'player_name', width: 140 },
    { title: '选手编号', dataIndex: 'player_no', width: 100 },
    { title: '机构', dataIndex: 'institution', width: 160 },
    {
      title: '来源',
      dataIndex: 'source',
      width: 110,
      render: (_, r) =>
        r.source === 'direct_card' ? <Tag color="gold">直通卡</Tag> : <Tag color="blue">积分前 20</Tag>,
    },
    { title: '总积分', dataIndex: 'total_points', width: 100 },
  ];

  return (
    <PageContainer title={false}>
      <Card
        size="small"
        title="晋级管理"
        extra={
          <ProFormSelect
            noStyle
            fieldProps={{ value: seasonId, onChange: (v: number) => setSeasonId(v), style: { minWidth: 150 } }}
            request={async () => (seasons ?? []).map((s) => ({ label: s.name, value: s.season_id }))}
          />
        }
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          title="直通卡：甲级赛每个榜单第 1 名、超级赛每个榜单前 2 名直通总决赛（自动标记，可手动增补/取消）；积分榜每榜前 20 名（扣除直通卡顺延）自动晋级。"
        />
        <Tabs
          items={[
            {
              key: 'cards',
              label: '直通卡',
              children: (
                <>
                  {access.canAdmin && (
                    <ModalForm
                      title="手动增补直通卡"
                      trigger={<Button type="primary" style={{ marginBottom: 12 }}>增补直通卡</Button>}
                      onFinish={async (values) => {
                        await grantQualificationCard(seasonId, {
                          player_id: Number(values.player_id),
                          board: String(values.board),
                          reason: String(values.reason),
                        });
                        message.success('已增补');
                        refreshCards();
                        return true;
                      }}
                    >
                      <ProFormText name="player_id" label="选手 ID" rules={[{ required: true }]} />
                      <ProFormText name="board" label="榜单（如 U11·古典舞）" rules={[{ required: true }]} />
                      <ProFormTextArea name="reason" label="增补原因（必填，留痕）" rules={[{ required: true }]} />
                    </ModalForm>
                  )}
                  <ProTable
                    rowKey="card_id"
                    size="small"
                    search={false}
                    options={false}
                    pagination={false}
                    actionRef={cardRef}
                    dataSource={cards ?? []}
                    columns={cardColumns}
                  />
                </>
              ),
            },
            {
              key: 'roster',
              label: '总决赛资格名单',
              children: (
                <>
                  {access.canAdmin && (
                    <ModalForm
                      title="手动调整资格名单"
                      trigger={<Button style={{ marginBottom: 12 }}>手动调整（增补/移除）</Button>}
                      onFinish={async (values) => {
                        await adjustFinalsRoster(seasonId, {
                          player_id: Number(values.player_id),
                          action: values.action as 'add' | 'remove',
                          reason: String(values.reason),
                        });
                        message.success('已调整，操作留痕');
                        refreshRoster();
                        return true;
                      }}
                    >
                      <ProFormText name="player_id" label="选手 ID" rules={[{ required: true }]} />
                      <ProFormSelect
                        name="action"
                        label="动作"
                        rules={[{ required: true }]}
                        options={[
                          { label: '增补', value: 'add' },
                          { label: '移除', value: 'remove' },
                        ]}
                      />
                      <ProFormTextArea name="reason" label="原因（必填，留痕）" rules={[{ required: true }]} />
                    </ModalForm>
                  )}
                  <ProTable
                    rowKey="player_id"
                    size="small"
                    search={false}
                    options={false}
                    pagination={false}
                    actionRef={rosterRef}
                    dataSource={roster ?? []}
                    columns={rosterColumns}
                  />
                </>
              ),
            },
          ]}
        />
      </Card>
    </PageContainer>
  );
};

export default Promotion;
