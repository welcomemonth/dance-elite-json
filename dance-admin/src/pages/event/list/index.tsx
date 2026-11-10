import { PlusOutlined } from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProTable } from '@ant-design/pro-components';
import { history, useAccess } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import { App, Button, Popconfirm, Space, Tag, Typography } from 'antd';
import React, { useRef } from 'react';
import {
  EVENT_LEVEL_LABELS,
  EVENT_STATUS_LABELS,
} from '@/services/youwumeng/labels';
import { formatDate, formatMoney } from '@/services/youwumeng/format';
import {
  generateAudioReviewToken,
  listEvents,
  offlineEvent,
  publishEvent,
} from '@/services/youwumeng/events';
import type { EventBrief, EventLevel, EventStatus } from '@/services/youwumeng/typings';

const STATUS_COLORS: Record<EventStatus, string> = {
  draft: 'default',
  unpublished: 'warning',
  registering: 'success',
  reg_closed: 'processing',
  ongoing: 'processing',
  finished: 'default',
  offline: 'error',
};

const EventList: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const actionRef = useRef<ActionType>(null);

  const { run: runPublish } = useService(publishEvent, {
    manual: true,
    onSuccess: () => {
      message.success('已发布，小程序端立即可见');
      actionRef.current?.reload();
    },
  });
  const { run: runOffline } = useService(offlineEvent, {
    manual: true,
    onSuccess: () => {
      message.success('已下架，小程序端立即不可见');
      actionRef.current?.reload();
    },
  });
  const { run: runGenToken } = useService(generateAudioReviewToken, {
    manual: true,
    onSuccess: (data) => {
      message.success(`音频审核链接：${data.url}`);
      actionRef.current?.reload();
    },
  });

  const columns: ProColumns<EventBrief>[] = [
    {
      title: '赛事名称',
      dataIndex: 'name',
      width: 280,
      render: (_, record) => (
        <Typography.Link
          onClick={() => history.push(`/event/detail/${record.event_id}`)}
        >
          {record.name}
        </Typography.Link>
      ),
    },
    {
      title: '级别',
      dataIndex: 'level',
      width: 100,
      valueEnum: Object.fromEntries(
        Object.entries(EVENT_LEVEL_LABELS).map(([value, label]) => [value, { text: label }]),
      ),
      render: (_, record) => <Tag>{EVENT_LEVEL_LABELS[record.level]}</Tag>,
    },
    { title: '城市', dataIndex: 'city', width: 80, search: false },
    {
      title: '比赛日期',
      dataIndex: 'event_date',
      width: 110,
      search: false,
      render: (_, record) => formatDate(record.event_date),
    },
    {
      title: '报名费',
      dataIndex: 'fee',
      width: 100,
      search: false,
      render: (_, record) => formatMoney(record.fee),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 100,
      valueEnum: Object.fromEntries(
        Object.entries(EVENT_STATUS_LABELS).map(([value, text]) => [value, { text }]),
      ),
      render: (_, record) => (
        <Tag color={STATUS_COLORS[record.status]}>
          {EVENT_STATUS_LABELS[record.status]}
        </Tag>
      ),
    },
    {
      title: '报名进度',
      dataIndex: 'groups',
      search: false,
      render: (_, record) => {
        const registered = record.groups.reduce((s, g) => s + g.registered, 0);
        const quota = record.groups.reduce((s, g) => s + g.quota, 0);
        const closed = record.groups.filter((g) => !g.enabled).length;
        return (
          <span>
            {registered}/{quota}
            {closed > 0 && <Tag style={{ marginLeft: 8 }}>已关闭组别 {closed}</Tag>}
          </span>
        );
      },
    },
    {
      title: '操作',
      valueType: 'option',
      width: 240,
      render: (_, record) => (
        <Space wrap size={4}>
          <Typography.Link
            onClick={() => history.push(`/event/detail/${record.event_id}`)}
          >
            详情
          </Typography.Link>
          {access.canAdmin && ['draft', 'unpublished'].includes(record.status) && (
            <Typography.Link
              onClick={() => history.push(`/event/edit/${record.event_id}`)}
            >
              编辑
            </Typography.Link>
          )}
          {access.canAdmin && record.status === 'unpublished' && (
            <Popconfirm
              title="发布赛事"
              description="发布后小程序端立即可见，是否继续？"
              onConfirm={() => runPublish(record.event_id)}
            >
              <Typography.Link>发布</Typography.Link>
            </Popconfirm>
          )}
          {access.canAdmin && ['registering', 'unpublished', 'reg_closed'].includes(record.status) && (
            <Popconfirm
              title="下架赛事"
              description="下架后小程序端立即不可见、不可报名；已支付报名不受影响。"
              onConfirm={() => runOffline(record.event_id)}
            >
              <Typography.Link type="danger">下架</Typography.Link>
            </Popconfirm>
          )}
          {access.canAdmin && (
            <Popconfirm
              title="音频审核链接"
              description={
                record.audio_review_token
                  ? '重新生成后旧链接立即失效，是否继续？'
                  : '生成该赛事的免登录音频审核 H5 链接？'
              }
              onConfirm={() => runGenToken(record.event_id)}
            >
              <Typography.Link>审核链接</Typography.Link>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  return (
    <PageContainer title={false}>
      <ProTable<EventBrief>
        headerTitle="赛事列表"
        rowKey="event_id"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 'auto' }}
        request={async (params) => {
          const data = await listEvents({
            status: params.status as EventStatus | undefined,
            level: params.level as EventLevel | undefined,
            keyword: params.name,
            page: params.current,
            page_size: params.pageSize,
          });
          return { data: data.list, total: data.total, success: true };
        }}
        pagination={{ defaultPageSize: 20 }}
        toolBarRender={() =>
          access.canAdmin
            ? [
                <Button
                  key="create"
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={() => history.push('/event/create')}
                >
                  新建赛事
                </Button>,
              ]
            : []
        }
      />
    </PageContainer>
  );
};

export default EventList;
