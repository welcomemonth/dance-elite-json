import { useAccess } from '@umijs/max';
import { PageContainer } from '@ant-design/pro-components';
import { useService } from '@/services/youwumeng/useService';
import {
  App,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Image,
  Row,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import React, { useState } from 'react';
import { useParams } from '@umijs/max';
import { formatDateTime, formatMoney } from '@/services/youwumeng/format';
import {
  DANCE_TYPE_LABELS,
  ORDER_SOURCE_LABELS,
  ORDER_STATUS_LABELS,
} from '@/services/youwumeng/labels';
import { getOrder, revealPhone } from '@/services/youwumeng/orders';

/**
 * 订单详情：报名信息 / 支付信息 / 资料信息 / 操作记录 四区。
 * 对应设计文档 3.2 节。
 */
const OrderDetail: React.FC = () => {
  const { orderNo } = useParams<{ orderNo: string }>();
  const { message } = App.useApp();
  const access = useAccess();
  const [revealedPhone, setRevealedPhone] = useState<string | null>(null);

  const { data, loading, error } = useService(
    () => getOrder(orderNo as string),
    { ready: !!orderNo },
  );

  const { run: runReveal, loading: revealing } = useService(revealPhone, {
    manual: true,
    onSuccess: (result) => {
      setRevealedPhone(result.contact_phone);
      message.warning('已查看明文手机号，本次操作已记录日志');
    },
  });

  if (!loading && (error || !data)) {
    return (
      <PageContainer title="订单详情">
        <Empty description="订单不存在或加载失败" />
      </PageContainer>
    );
  }

  const { order, registration, payments, materials, operation_logs: logs, remarks } = data ?? {};
  const materialItems = [
    { label: '证件照', value: materials?.photo_url, type: 'photo' as const },
    { label: '音频', value: materials?.audio_url, type: 'audio' as const },
    { label: '节目名称', value: materials?.program_name, type: 'text' as const },
    { label: '联系电话', value: materials?.contact_phone, type: 'text' as const },
  ];

  return (
    <PageContainer title={`订单详情 ${orderNo ?? ''}`} loading={loading && !data}>
      <Space direction="vertical" size={16} style={{ display: 'flex' }}>
        <Card title="报名信息" size="small">
          <Descriptions size="small" column={3} bordered>
            <Descriptions.Item label="赛事">
              {registration?.event_name}
            </Descriptions.Item>
            <Descriptions.Item label="组别">
              {registration?.level} · {DANCE_TYPE_LABELS[registration?.dance_type as 'classical'] ?? registration?.dance_type}
            </Descriptions.Item>
            <Descriptions.Item label="选手姓名">
              {registration?.player_name}
            </Descriptions.Item>
            <Descriptions.Item label="证件号（脱敏）">
              {registration?.id_card_masked ?? '无证件号'}
            </Descriptions.Item>
            <Descriptions.Item label="机构">{registration?.institution || '-'}</Descriptions.Item>
            <Descriptions.Item label="联系电话">
              <Space size={4}>
                <span>{revealedPhone ?? registration?.contact_phone}</span>
                {access.canRevealPhone && !revealedPhone && (
                  <Button
                    type="link"
                    size="small"
                    loading={revealing}
                    onClick={() => runReveal(orderNo as string)}
                  >
                    明文查看
                  </Button>
                )}
              </Space>
            </Descriptions.Item>
            <Descriptions.Item label="资料截止时间">
              {formatDateTime(registration?.material_deadline)}
            </Descriptions.Item>
            <Descriptions.Item label="来源">
              {ORDER_SOURCE_LABELS[order?.source as 'miniapp'] ?? order?.source}
            </Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag>{ORDER_STATUS_LABELS[order?.status as 'paid'] ?? order?.status}</Tag>
            </Descriptions.Item>
          </Descriptions>
        </Card>

        <Card title="支付信息" size="small">
          <Descriptions size="small" column={3} bordered>
            <Descriptions.Item label="订单号">{order?.order_no}</Descriptions.Item>
            <Descriptions.Item label="微信支付单号">
              {order?.transaction_id || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="金额">
              {formatMoney(order?.amount ?? 0)}
            </Descriptions.Item>
            <Descriptions.Item label="支付时间">
              {formatDateTime(order?.paid_at)}
            </Descriptions.Item>
            <Descriptions.Item label="取消原因">
              {order?.cancel_reason ?? '-'}
            </Descriptions.Item>
          </Descriptions>
          <Table
            style={{ marginTop: 12 }}
            rowKey={(r) => `${r.type}-${r.transaction_id}`}
            size="small"
            pagination={false}
            dataSource={payments ?? []}
            columns={[
              { title: '类型', dataIndex: 'type', render: (v: string) => (v === 'pay' ? '支付' : '退款') },
              { title: '微信支付单号', dataIndex: 'transaction_id' },
              { title: '退款单号', dataIndex: 'refund_id', render: (v: string) => v || '-' },
              {
                title: '金额',
                dataIndex: 'amount',
                render: (v: number) => formatMoney(v),
              },
              { title: '状态', dataIndex: 'status' },
              {
                title: '时间',
                dataIndex: 'notified_at',
                render: (v: string) => formatDateTime(v),
              },
            ]}
          />
        </Card>

        <Card
          title="资料信息"
          size="small"
          extra={
            materials?.readonly ? (
              <Tag color="warning">资料已截止，整体只读</Tag>
            ) : (
              <Tag color="success">资料可修改</Tag>
            )
          }
        >
          <Descriptions size="small" column={2} bordered>
            {materialItems.map((item) => (
              <Descriptions.Item key={item.label} label={item.label}>
                {item.type === 'photo' &&
                  (item.value ? (
                    <Image src={item.value} width={64} height={80} style={{ objectFit: 'cover' }} />
                  ) : (
                    <Tag>未传</Tag>
                  ))}
                {item.type === 'audio' &&
                  (item.value ? (
                    // biome-ignore lint/a11y/useMediaCaption: 舞蹈音频为纯音乐，无字幕轨
                    <audio controls src={item.value} style={{ width: 260, height: 32 }} />
                  ) : (
                    <Tag>未传</Tag>
                  ))}
                {item.type === 'text' && (item.value ?? <Tag>未传</Tag>)}
              </Descriptions.Item>
            ))}
            <Descriptions.Item label="最后修改时间">
              {formatDateTime(materials?.updated_at)}
            </Descriptions.Item>
          </Descriptions>
        </Card>

        <Card title="操作记录" size="small">
          <Table
            rowKey={(r) => `${r.action}-${r.created_at}`}
            size="small"
            pagination={false}
            dataSource={logs ?? []}
            columns={[
              { title: '动作', dataIndex: 'action' },
              {
                title: '状态流转',
                render: (_, r) =>
                  `${r.from_status ?? '-'} → ${r.to_status ?? '-'}`,
              },
              { title: '触发方', dataIndex: 'operator' },
              {
                title: '时间',
                dataIndex: 'created_at',
                render: (v: string) => formatDateTime(v),
              },
            ]}
          />
        </Card>

        <Card title="备注" size="small">
          {remarks && remarks.length > 0 ? (
            remarks.map((r) => (
              <Typography.Paragraph key={r.id} style={{ marginBottom: 4 }}>
                {r.content}（{r.operator} · {formatDateTime(r.created_at)}）
              </Typography.Paragraph>
            ))
          ) : (
            <Typography.Text type="secondary">暂无备注</Typography.Text>
          )}
        </Card>
      </Space>
    </PageContainer>
  );
};

export default OrderDetail;
