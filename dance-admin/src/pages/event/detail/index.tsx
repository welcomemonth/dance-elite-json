import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PageContainer, ProDescriptions, ProTable } from '@ant-design/pro-components';
import { history, useAccess, useParams } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import {
  Alert,
  App,
  Button,
  Card,
  Col,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Space,
  Steps,
  Table,
  Tabs,
  Tag,
  Typography,
} from 'antd';
import React, { useRef, useState } from 'react';
import {
  closeGroup,
  generateAudioReviewToken,
  getEvent,
  publishEvent,
  reopenGroup,
  setGroupQuota,
} from '@/services/youwumeng/events';
import {
  EVENT_LEVEL_LABELS,
  EVENT_STATUS_LABELS,
  DANCE_TYPE_LABELS,
} from '@/services/youwumeng/labels';
import { formatDate, formatDateTime, formatMoney } from '@/services/youwumeng/format';
import type { EventBrief, EventGroup } from '@/services/youwumeng/typings';

/** 赛事详情页：顶部信息 + 时间线 + 音频审核链接 + Tab（设计文档 2.3） */
const EventDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const eventId = Number(id);
  const { message } = App.useApp();
  const access = useAccess();
  const [quotaTarget, setQuotaTarget] = useState<EventGroup | null>(null);
  const [quota, setQuota] = useState<number>(0);
  const actionRef = useRef<ActionType>(null);

  const { data: event, loading, refresh } = useService(() => getEvent(eventId), {
    refreshDeps: [eventId],
  });

  const reload = () => refresh();

  if (!loading && !event) {
    return (
      <PageContainer title="赛事不存在">
        <Alert type="error" message="赛事不存在或已被删除" showIcon />
      </PageContainer>
    );
  }

  const timeline = event?.timeline;
  const timelineItems = timeline
    ? [
        { title: '报名开始', time: timeline.reg_start_at },
        { title: '截止报名', time: timeline.reg_end_at },
        { title: '抽签', time: timeline.lottery_at },
        { title: '核对舞蹈音频', time: timeline.material_check_at },
        { title: '开始比赛', time: timeline.start_at },
      ]
    : [];
  const currentStep = timelineItems.filter((t) => new Date(t.time).getTime() <= Date.now()).length;

  const groupColumns: ProColumns<EventGroup>[] = [
    {
      title: '组别·舞种',
      render: (_, g) => `${g.level}·${DANCE_TYPE_LABELS[g.dance_type]}`,
    },
    { title: '名额', dataIndex: 'quota', width: 80 },
    { title: '已支付', dataIndex: 'registered', width: 90 },
    {
      title: '剩余',
      width: 80,
      render: (_, g) => Math.max(0, g.quota - g.registered),
    },
    {
      title: '状态',
      width: 90,
      render: (_, g) =>
        g.enabled ? <Tag color="success">开放中</Tag> : <Tag>已关闭</Tag>,
    },
    {
      title: '操作',
      valueType: 'option',
      width: 200,
      render: (_, g) =>
        access.canAdmin ? (
          <Space size={4}>
            <Typography.Link
              onClick={() => {
                setQuotaTarget(g);
                setQuota(g.quota);
              }}
            >
              加名额
            </Typography.Link>
            {g.enabled ? (
              <Popconfirm
                title="关闭组别"
                description="小程序端该组别立即不可报名，已支付报名不受影响。"
                onConfirm={async () => {
                  await closeGroup(eventId, g.group_id);
                  message.success('已关闭');
                  reload();
                }}
              >
                <Typography.Link type="danger">关闭组别</Typography.Link>
              </Popconfirm>
            ) : (
              <Popconfirm
                title="重新开放组别"
                description="恢复后名额不变，小程序端立即恢复可报名。"
                onConfirm={async () => {
                  await reopenGroup(eventId, g.group_id);
                  message.success('已重新开放');
                  reload();
                }}
              >
                <Typography.Link>重新开放</Typography.Link>
              </Popconfirm>
            )}
          </Space>
        ) : null,
    },
  ];

  return (
    <PageContainer
      title={event?.name ?? '加载中…'}
      loading={loading}
      extra={
        event && ['unpublished', 'draft'].includes(event.status) && access.canAdmin ? (
          <Button
            type="primary"
            onClick={() =>
              Modal.confirm({
                title: '发布赛事',
                content: '发布后小程序端立即可见，是否继续？',
                onOk: async () => {
                  await publishEvent(event.event_id);
                  message.success('已发布');
                  reload();
                },
              })
            }
          >
            发布
          </Button>
        ) : undefined
      }
    >
      {event && (
        <>
          <Card size="small" style={{ marginBottom: 16 }}>
            <ProDescriptions
              column={{ xs: 1, sm: 2, md: 3 }}
              dataSource={event}
              columns={[
                { title: '级别', render: () => <Tag>{EVENT_LEVEL_LABELS[event.level]}</Tag> },
                { title: '状态', render: () => <Tag color="processing">{EVENT_STATUS_LABELS[event.status]}</Tag> },
                { title: '举办城市', dataIndex: 'city' },
                { title: '比赛地点', dataIndex: 'venue' },
                { title: '比赛日期', render: () => formatDate(event.event_date) },
                { title: '报名费', render: () => formatMoney(event.fee) },
                {
                  title: '音频审核链接',
                  span: 3,
                  render: () => (
                    <Space>
                      {event.audio_review_token ? (
                        <>
                          <Typography.Text code copyable>
                            {`/audio-review/${event.audio_review_token}`}
                          </Typography.Text>
                          <Popconfirm
                            title="重新生成"
                            description="重新生成后旧链接立即失效，是否继续？"
                            onConfirm={async () => {
                              const r = await generateAudioReviewToken(event.event_id);
                              message.success(`新链接：${r.url}`);
                              reload();
                            }}
                          >
                            <Typography.Link>重新生成</Typography.Link>
                          </Popconfirm>
                        </>
                      ) : (
                        <Typography.Link
                          onClick={async () => {
                            const r = await generateAudioReviewToken(event.event_id);
                            message.success(`已生成：${r.url}`);
                            reload();
                          }}
                        >
                          生成音频审核链接
                        </Typography.Link>
                      )}
                    </Space>
                  ),
                },
              ]}
            />
            <Steps
              current={currentStep}
              size="small"
              style={{ marginTop: 16 }}
              items={timelineItems.map((t) => ({
                title: t.title,
                description: formatDateTime(t.time),
              }))}
            />
          </Card>

          <Tabs
            items={[
              {
                key: 'overview',
                label: '报名概览',
                children: (
                  <ProTable<EventGroup>
                    rowKey="group_id"
                    size="small"
                    search={false}
                    options={false}
                    pagination={false}
                    actionRef={actionRef}
                    dataSource={event.groups}
                    columns={groupColumns}
                  />
                ),
              },
              {
                key: 'orders',
                label: '报名名单',
                children: (
                  <Space direction="vertical">
                    <Typography.Text type="secondary">
                      该赛事全部有效报名请在订单管理中查看（自动按本赛事筛选）。
                    </Typography.Text>
                    <Button onClick={() => history.push(`/order/list?event_id=${event.event_id}`)}>
                      前往订单管理
                    </Button>
                  </Space>
                ),
              },
              {
                key: 'waitlist',
                label: '候补队列',
                children: (
                  <Button onClick={() => history.push(`/operation/waitlist?event_id=${event.event_id}`)}>
                    前往候补管理
                  </Button>
                ),
              },
              {
                key: 'materials',
                label: '资料完成度',
                children: (
                  <Button onClick={() => history.push(`/operation/materials?event_id=${event.event_id}`)}>
                    前往资料催收
                  </Button>
                ),
              },
              {
                key: 'refunds',
                label: '退款记录',
                children: (
                  <Button onClick={() => history.push(`/order/refunds?event_id=${event.event_id}`)}>
                    前往退款工单
                  </Button>
                ),
              },
            ]}
          />

          <Modal
            title={`加名额：${quotaTarget?.level}·${quotaTarget ? DANCE_TYPE_LABELS[quotaTarget.dance_type] : ''}`}
            open={!!quotaTarget}
            onCancel={() => setQuotaTarget(null)}
            onOk={async () => {
              if (!quotaTarget) return;
              await setGroupQuota(eventId, quotaTarget.group_id, quota);
              message.success('已生效，小程序端剩余名额即时刷新');
              setQuotaTarget(null);
              reload();
            }}
          >
            <Alert
              type="info"
              showIcon
              title="名额只能增加，不得小于当前已支付人数；增加即时生效。"
              style={{ marginBottom: 12 }}
            />
            <InputNumber
              min={quotaTarget?.registered ?? 1}
              value={quota}
              onChange={(v) => setQuota(v ?? 0)}
              style={{ width: '100%' }}
            />
          </Modal>
        </>
      )}
    </PageContainer>
  );
};

export default EventDetail;
