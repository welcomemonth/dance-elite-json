import { history, useAccess, useParams } from '@umijs/max';
import { PageContainer } from '@ant-design/pro-components';
import { useService } from '@/services/youwumeng/useService';
import {
  Alert,
  App,
  Button,
  Card,
  Col,
  Descriptions,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Space,
  Tag,
  Typography,
} from 'antd';
import React, { useEffect, useState } from 'react';
import { formatDateTime, formatMoney } from '@/services/youwumeng/format';
import { ORDER_STATUS_LABELS } from '@/services/youwumeng/labels';
import {
  approveRefundTicket,
  getRefundTicket,
  rejectRefundTicket,
} from '@/services/youwumeng/refunds';

const POLICY_TEXT = {
  lottery_before: '当前处于抽签前：可无偿退赛，全额原路退回。',
  lottery_after: '当前处于抽签后：个人原因退赛扣除 50%；伤病凭医院诊断证明可顺延不扣费。',
} as const;

/**
 * 退款工单处理页：左摘要 + 右处理区（设计文档 4.2）。
 * 同意退款为资金操作，二次确认（金额 + 订单 + 选手三要素）。
 */
const RefundDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const { message } = App.useApp();
  const access = useAccess();
  const [form] = Form.useForm<{ amount: number; adjust_reason?: string }>();
  const [rejectForm] = Form.useForm<{ reject_reason: string }>();
  const [rejectOpen, setRejectOpen] = useState(false);
  const [result, setResult] = useState<{ type: 'approved' | 'rejected' | 'failed'; detail: string } | null>(null);

  const ticketId = Number(id);
  const { data, loading, error, refresh } = useService(
    () => getRefundTicket(ticketId),
    { ready: Number.isFinite(ticketId) },
  );

  const suggestAmount = data?.suggest_amount ?? 0;
  const [adjustReasonRequired, setAdjustReasonRequired] = useState(false);

  useEffect(() => {
    if (data) {
      form.setFieldsValue({ amount: data.suggest_amount });
      setAdjustReasonRequired(false);
    }
  }, [data, form]);

  const handleApprove = async () => {
    const values = await form.validateFields();
    if (values.amount !== suggestAmount && !values.adjust_reason?.trim()) {
      setAdjustReasonRequired(true);
      return;
    }
    const detail = `订单 ${data?.order_no}，选手 ${data?.player_name}，退款金额 ${formatMoney(values.amount)}`;
    Modal.confirm({
      title: '确认同意退款？',
      content: `请核对三要素：${detail}。确认后将调用微信支付退款 API 原路退回，操作不可撤销。`,
      okText: '确认退款',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          const res = await approveRefundTicket(ticketId, {
            amount: values.amount,
            adjust_reason: values.adjust_reason?.trim() || undefined,
          });
          setResult({ type: 'approved', detail: `退款成功，微信退款单号：${res.refund_id}` });
          message.success('退款成功');
          refresh();
        } catch (e) {
          // 41014 退款失败：留在页面提示可重试
          setResult({
            type: 'failed',
            detail: e instanceof Error ? e.message : '微信支付退款失败',
          });
          refresh();
        }
      },
    });
  };

  const handleReject = async () => {
    const values = await rejectForm.validateFields();
    await rejectRefundTicket(ticketId, { reject_reason: values.reject_reason });
    setResult({ type: 'rejected', detail: `已驳回：${values.reject_reason}` });
    setRejectOpen(false);
    message.success('已驳回，订单状态回退为已支付');
    refresh();
  };

  if (!loading && (error || !data)) {
    return (
      <PageContainer title="退款工单">
        <Empty description="工单不存在或加载失败" />
      </PageContainer>
    );
  }

  const actionable = ['pending', 'refund_failed'].includes(data?.status ?? '');
  const showActions = actionable && access.canAdmin;

  return (
    <PageContainer title={`退款工单 ${data?.ticket_no ?? ''}`} loading={loading && !data}>
      <Row gutter={16}>
        <Col span={14}>
          <Space direction="vertical" size={16} style={{ display: 'flex' }}>
            <Card title="工单信息" size="small">
              <Descriptions size="small" column={2} bordered>
                <Descriptions.Item label="工单号">{data?.ticket_no}</Descriptions.Item>
                <Descriptions.Item label="状态">
                  <Tag color={data?.status === 'pending' ? 'warning' : data?.status === 'approved' ? 'success' : data?.status === 'refund_failed' ? 'error' : 'default'}>
                    {data?.status}
                  </Tag>
                </Descriptions.Item>
                <Descriptions.Item label="诉求摘要">{data?.reason}</Descriptions.Item>
                <Descriptions.Item label="登记来源">{data?.applicant}</Descriptions.Item>
                <Descriptions.Item label="登记时间">
                  {formatDateTime(data?.created_at)}
                </Descriptions.Item>
                <Descriptions.Item label="处理人">
                  {data?.handler_name ?? '-'}
                </Descriptions.Item>
              </Descriptions>
              <Alert
                style={{ marginTop: 12 }}
                type="info"
                showIcon
                title="客服登记内容"
                description={data?.content}
              />
            </Card>

            <Card title="关联订单" size="small">
              {data?.order ? (
                <Descriptions size="small" column={2} bordered>
                  <Descriptions.Item label="订单号">
                    <Typography.Link
                      onClick={() => history.push(`/order/detail/${data.order?.order_no}`)}
                    >
                      {data.order.order_no}
                    </Typography.Link>
                  </Descriptions.Item>
                  <Descriptions.Item label="赛事">{data.order.event_name}</Descriptions.Item>
                  <Descriptions.Item label="组别">
                    {data.order.level} · {data.order.dance_type}
                  </Descriptions.Item>
                  <Descriptions.Item label="选手">{data.order.player_name}</Descriptions.Item>
                  <Descriptions.Item label="实付金额">
                    {formatMoney(data.order.amount)}
                  </Descriptions.Item>
                  <Descriptions.Item label="订单状态">
                    {ORDER_STATUS_LABELS[data.order.status]}
                  </Descriptions.Item>
                </Descriptions>
              ) : (
                <Empty description="关联订单不存在" />
              )}
            </Card>

            <Card title="处理留痕" size="small">
              {data?.history.map((h) => (
                <Typography.Paragraph key={`${h.created_at}-${h.action}`} style={{ marginBottom: 4 }}>
                  {formatDateTime(h.created_at)} · {h.operator} · {h.action}：{h.detail}
                </Typography.Paragraph>
              ))}
            </Card>
          </Space>
        </Col>

        <Col span={10}>
          <Card title="处理区" size="small">
            <Alert
              type={data?.policy_stage === 'lottery_before' ? 'success' : 'warning'}
              showIcon
              title="退费政策"
              description={POLICY_TEXT[data?.policy_stage ?? 'lottery_before']}
              style={{ marginBottom: 16 }}
            />

            {result && (
              <Alert
                style={{ marginBottom: 16 }}
                type={result.type === 'approved' ? 'success' : result.type === 'rejected' ? 'info' : 'error'}
                showIcon
                title={result.type === 'approved' ? '退款成功' : result.type === 'rejected' ? '已驳回' : '退款失败'}
                description={result.detail}
              />
            )}

            {data?.status === 'refund_failed' && (
              <Alert
                style={{ marginBottom: 16 }}
                type="error"
                showIcon
                title="上一笔退款调用失败"
                description="可修正金额后重试，或驳回工单。"
              />
            )}

            {showActions ? (
              <Form form={form} layout="vertical">
                <Form.Item
                  name="amount"
                  label="退款金额（分）"
                  rules={[{ required: true, message: '请填写退款金额' }]}
                  extra={`按政策建议全额/半额：${formatMoney(suggestAmount)}（实付 ${formatMoney(data?.paid_amount ?? 0)}）`}
                >
                  <InputNumber
                    style={{ width: '100%' }}
                    min={1}
                    max={data?.paid_amount ?? undefined}
                    onChange={(v) => setAdjustReasonRequired(v !== undefined && v !== suggestAmount)}
                  />
                </Form.Item>
                <Form.Item
                  name="adjust_reason"
                  label="调整原因（金额与政策建议不一致时必填）"
                  validateStatus={adjustReasonRequired ? 'error' : ''}
                  help={adjustReasonRequired ? '金额已调整，请填写调整原因' : undefined}
                >
                  <Input.TextArea rows={2} placeholder="如：伤病凭诊断证明全额退" />
                </Form.Item>
                <Space>
                  <Button type="primary" danger onClick={handleApprove}>
                    同意退款
                  </Button>
                  <Button onClick={() => setRejectOpen(true)}>驳回</Button>
                </Space>
              </Form>
            ) : (
              <Alert
                type="info"
                showIcon
                title={
                  actionable
                    ? '当前账号无处理权限（客服只读）'
                    : `工单已处理（状态：${data?.status}）`
                }
              />
            )}
          </Card>
        </Col>
      </Row>

      <Modal
        title="驳回退款"
        open={rejectOpen}
        onCancel={() => setRejectOpen(false)}
        onOk={handleReject}
        okText="确认驳回"
        okButtonProps={{ danger: true }}
      >
        <Form form={rejectForm} layout="vertical">
          <Form.Item
            name="reject_reason"
            label="驳回原因"
            rules={[{ required: true, message: '驳回原因必填' }]}
          >
            <Input.TextArea rows={3} maxLength={200} showCount placeholder="如：已过退费期" />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
};

export default RefundDetail;
