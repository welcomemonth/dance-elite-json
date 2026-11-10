import type { ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProDescriptions, ProTable } from '@ant-design/pro-components';
import { history, useAccess } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import {
  App,
  Avatar,
  Button,
  Descriptions,
  Drawer,
  Popconfirm,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
} from 'antd';
import React, { useState } from 'react';
import { formatDateTime } from '@/services/youwumeng/format';
import {
  AWARD_LABELS,
  DANCE_TYPE_LABELS,
  ORDER_STATUS_LABELS,
} from '@/services/youwumeng/labels';
import {
  getPlayer,
  listPlayers,
  listUsers,
  mergePlayer,
} from '@/services/youwumeng/users';
import type { PlayerBrief, PlayerDetail, WxUser } from '@/services/youwumeng/typings';

const ID_TYPE_LABELS: Record<PlayerBrief['id_type'], string> = {
  id_card: '大陆身份证',
  passport: '护照',
  hk_macau: '港澳台证件',
  other: '其他',
};

const GENDER_LABELS: Record<PlayerBrief['gender'], string> = {
  male: '男',
  female: '女',
};

function PlayerArchives() {
  const { message } = App.useApp();
  const actionRef = React.useRef<{ reload: () => void }>(null);
  const [detailId, setDetailId] = useState<number | null>(null);

  const { data: detail, loading: detailLoading } = useService(
    () => getPlayer(detailId!),
    { ready: detailId !== null, refreshDeps: [detailId] },
  );
  const { run: runMerge } = useService(mergePlayer, {
    manual: true,
    onSuccess: () => {
      message.success('已合并档案（操作已留痕）');
      setDetailId(null);
      actionRef.current?.reload();
    },
  });

  const columns: ProColumns<PlayerBrief>[] = [
    { title: '选手编号', dataIndex: 'player_no', width: 100 },
    { title: '姓名', dataIndex: 'name', width: 120 },
    {
      title: '证件类型',
      dataIndex: 'id_type',
      width: 110,
      render: (_, r) => ID_TYPE_LABELS[r.id_type],
    },
    {
      title: '证件号',
      dataIndex: 'id_card_masked',
      width: 200,
      render: (_, r) => r.id_card_masked ?? '—',
    },
    {
      title: '出生日期',
      dataIndex: 'birth_date',
      width: 110,
      search: false,
    },
    {
      title: '性别',
      dataIndex: 'gender',
      width: 70,
      search: false,
      render: (_, r) => GENDER_LABELS[r.gender],
    },
    { title: '机构', dataIndex: 'institution', width: 140, search: false, render: (v) => v || '—' },
    { title: '报名次数', dataIndex: 'registration_count', width: 90, search: false },
    { title: '成绩数', dataIndex: 'result_count', width: 80, search: false },
    {
      title: '状态',
      width: 90,
      search: false,
      render: (_, r) =>
        r.merged_into ? <Tag color="default">已合并</Tag> : <Tag color="success">正常</Tag>,
    },
    {
      title: '操作',
      valueType: 'option',
      width: 80,
      render: (_, r) => (
        <Typography.Link onClick={() => setDetailId(r.player_id)}>详情</Typography.Link>
      ),
    },
  ];

  return (
    <>
      <ProTable<PlayerBrief>
        rowKey="player_id"
        actionRef={actionRef as never}
        columns={columns}
        search={{ labelWidth: 'auto' }}
        request={async (params) => {
          const data = await listPlayers({
            keyword: params.keyword as string,
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        rowClassName={(r) => (r.merged_into ? 'ant-table-row-disabled' : '')}
      />
      <Drawer
        title={detail ? `选手档案：${detail.name}（${detail.player_no}）` : '选手档案'}
        width={720}
        open={detailId !== null}
        onClose={() => setDetailId(null)}
      >
        {detail && (
          <>
            <ProDescriptions
              loading={detailLoading}
              column={2}
              dataSource={detail}
              columns={[
                { title: '选手编号', dataIndex: 'player_no' },
                { title: '姓名', dataIndex: 'name' },
                { title: '证件类型', dataIndex: 'id_type', render: () => ID_TYPE_LABELS[detail.id_type] },
                { title: '证件号', dataIndex: 'id_card_masked', render: (v) => (v as string) ?? '—' },
                { title: '出生日期', dataIndex: 'birth_date' },
                { title: '性别', dataIndex: 'gender', render: () => GENDER_LABELS[detail.gender] },
                { title: '机构', dataIndex: 'institution', render: (v) => (v as string) || '—' },
              ]}
            />
            <Typography.Title level={5} style={{ marginTop: 24 }}>
              历史报名（跨微信聚合）
            </Typography.Title>
            <Table
              rowKey="order_no"
              size="small"
              pagination={false}
              dataSource={detail.registrations}
              columns={[
                {
                  title: '订单号',
                  dataIndex: 'order_no',
                  render: (v) => (
                    <Typography.Link
                      onClick={() => history.push(`/order/detail/${v}`)}
                    >
                      {v as string}
                    </Typography.Link>
                  ),
                },
                { title: '赛事', dataIndex: 'event_name' },
                {
                  title: '组别/舞种',
                  render: (_, r) =>
                    `${r.level}·${DANCE_TYPE_LABELS[r.dance_type]}`,
                },
                {
                  title: '状态',
                  dataIndex: 'status',
                  render: (_, r) => (
                    <Tag>{ORDER_STATUS_LABELS[r.status as keyof typeof ORDER_STATUS_LABELS] ?? r.status}</Tag>
                  ),
                },
                {
                  title: '报名时间',
                  dataIndex: 'created_at',
                  render: (v) => formatDateTime(v as string),
                },
              ]}
            />
            <Typography.Title level={5} style={{ marginTop: 24 }}>
              成绩与积分汇总
            </Typography.Title>
            <Table
              rowKey={(r) => `${r.event_name}-${r.award}`}
              size="small"
              pagination={false}
              dataSource={detail.results_summary}
              columns={[
                { title: '奖项', dataIndex: 'award', render: (v) => (AWARD_LABELS as Record<string, string>)[v as string] ?? (v as string) },
                { title: '赛事', dataIndex: 'event_name' },
                { title: '积分', dataIndex: 'points' },
                { title: '时间', dataIndex: 'created_at', render: (v) => formatDateTime(v as string) },
              ]}
            />
            <Descriptions size="small" column={3} style={{ marginTop: 16 }}>
              <Descriptions.Item label="赛季">{detail.points_summary.season_name}</Descriptions.Item>
              <Descriptions.Item label="总积分">{detail.points_summary.total_points}</Descriptions.Item>
              <Descriptions.Item label="计分场次">{detail.points_summary.event_count}</Descriptions.Item>
            </Descriptions>
            {detail.duplicates.length > 0 && (
              <>
                <Typography.Title level={5} style={{ marginTop: 24 }}>
                  重复档案（同证件号，可合并）
                </Typography.Title>
                <Space direction="vertical" style={{ width: '100%' }}>
                  {detail.duplicates.map((d) => (
                    <Space key={d.player_id}>
                      <Typography.Text>
                        {d.name}（{d.player_no}，创建于 {formatDateTime(d.created_at)}）
                      </Typography.Text>
                      <Popconfirm
                        title="合并档案"
                        description={`将 ${d.name} 的档案合并到 ${detail.name}？此敏感操作将留痕。`}
                        onConfirm={() => runMerge(d.player_id, detail.player_id)}
                      >
                        <Button size="small" danger>
                          合并到当前档案
                        </Button>
                      </Popconfirm>
                    </Space>
                  ))}
                </Space>
              </>
            )}
          </>
        )}
      </Drawer>
    </>
  );
}

function WxUsers() {
  const columns: ProColumns<WxUser>[] = [
    {
      title: '头像',
      dataIndex: 'avatar_url',
      width: 70,
      search: false,
      render: (_, r) => <Avatar src={r.avatar_url} />,
    },
    { title: '微信昵称', dataIndex: 'nickname', width: 140 },
    { title: '手机号', dataIndex: 'phone_masked', width: 130, search: false },
    { title: '报名次数', dataIndex: 'registration_count', width: 90, search: false },
    {
      title: '状态',
      dataIndex: 'status',
      width: 90,
      valueEnum: {
        active: { text: '正常' },
        blocked: { text: '已封禁' },
      },
      render: (_, r) => (
        <Tag color={r.status === 'active' ? 'success' : 'error'}>
          {r.status === 'active' ? '正常' : '已封禁'}
        </Tag>
      ),
    },
    {
      title: '注册时间',
      dataIndex: 'created_at',
      width: 150,
      search: false,
      render: (_, r) => formatDateTime(r.created_at),
    },
    {
      title: '最近活跃',
      dataIndex: 'last_login_at',
      width: 150,
      search: false,
      render: (_, r) => formatDateTime(r.last_login_at),
    },
  ];

  return (
    <ProTable<WxUser>
      rowKey="id"
      columns={columns}
      search={{ labelWidth: 'auto' }}
      request={async (params) => {
        const data = await listUsers({
          keyword: params.keyword as string,
          status: params.status as 'active' | 'blocked' | undefined,
          page: params.current,
          page_size: params.pageSize,
        });
        return { data: data.list, total: data.total, success: true };
      }}
      pagination={{ defaultPageSize: 20 }}
    />
  );
}

/** 选手/用户管理：选手档案（证件号维度）+ 微信用户两个视图 */
const UserContentPlayers: React.FC = () => {
  void useAccess(); // 本页只读，无写操作
  return (
    <PageContainer title={false}>
      <Tabs
        items={[
          { key: 'players', label: '选手档案', children: <PlayerArchives /> },
          { key: 'users', label: '微信用户', children: <WxUsers /> },
        ]}
      />
    </PageContainer>
  );
};

export default UserContentPlayers;
