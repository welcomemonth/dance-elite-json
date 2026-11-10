import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  DrawerForm,
  ModalForm,
  PageContainer,
  ProForm,
  ProFormDatePicker,
  ProFormDigit,
  ProFormSelect,
  ProFormText,
  ProTable,
} from '@ant-design/pro-components';
import { useAccess } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import {
  Alert,
  App,
  Button,
  Card,
  DatePicker,
  Form,
  Input,
  Modal,
  Space,
  Statistic,
  Tag,
  Typography,
} from 'antd';
import React, { useRef, useState } from 'react';
import { formatDateTime, formatMoney } from '@/services/youwumeng/format';
import { listEvents } from '@/services/youwumeng/events';
import {
  adjustPrizeStatement,
  getPrizeRules,
  listPrizeStatements,
  listSeasons,
  payPrizeStatement,
  putPrizeRules,
} from '@/services/youwumeng/seasons';
import type { PrizeRules, PrizeStatementItem } from '@/services/youwumeng/typings';

/** 奖金管理（设计文档 8.6）：规则配置 + 核算单 + 发放登记 */
const Prizes: React.FC = () => {
  const { message } = App.useApp();
  const access = useAccess();
  const [seasonId, setSeasonId] = useState(1);
  const [eventId, setEventId] = useState<number | undefined>();
  const [rulesOpen, setRulesOpen] = useState(false);
  const [payTarget, setPayTarget] = useState<PrizeStatementItem | null>(null);
  const [adjustTarget, setAdjustTarget] = useState<PrizeStatementItem | null>(null);
  const [adjustAmount, setAdjustAmount] = useState(0);
  const [adjustReason, setAdjustReason] = useState('');
  const actionRef = useRef<ActionType>(null);
  const [payForm] = Form.useForm();

  const { data: seasons } = useService(listSeasons);
  const { data: events } = useService(() => listEvents({ page: 1, page_size: 100 }));
  const { data, refresh } = useService(
    () => listPrizeStatements({ season_id: seasonId, event_id: eventId, page: 1, page_size: 100 }),
    { refreshDeps: [seasonId, eventId] },
  );

  const items = data?.list ?? [];
  const summary = data?.summary;

  const columns: ProColumns<PrizeStatementItem>[] = [
    { title: '赛事', dataIndex: 'event_name', width: 240 },
    { title: '选手', dataIndex: 'player_name', width: 120, render: (_, r) => r.player_name || '-' },
    { title: '机构', dataIndex: 'institution', width: 150, render: (_, r) => r.institution || '-' },
    { title: '奖项', dataIndex: 'award', width: 120 },
    {
      title: '金额',
      dataIndex: 'amount',
      width: 120,
      render: (_, r) =>
        r.adjusted_amount ? (
          <Space size={4}>
            <Typography.Text delete>{formatMoney(r.amount)}</Typography.Text>
            <Typography.Text type="warning">{formatMoney(r.adjusted_amount)}</Typography.Text>
          </Space>
        ) : (
          formatMoney(r.amount)
        ),
    },
    {
      title: '调整原因',
      dataIndex: 'adjust_reason',
      render: (_, r) => r.adjust_reason ?? '-',
    },
    {
      title: '发放状态',
      dataIndex: 'paid',
      width: 100,
      render: (_, r) =>
        r.paid ? <Tag color="success">已发放</Tag> : <Tag color="warning">待发放</Tag>,
    },
    { title: '领取人', dataIndex: 'payee', width: 100, render: (_, r) => r.payee ?? '-' },
    { title: '发放时间', dataIndex: 'paid_at', width: 150, render: (_, r) => formatDateTime(r.paid_at) },
    {
      title: '操作',
      valueType: 'option',
      width: 150,
      render: (_, r) =>
        access.canAdmin && !r.paid ? (
          <Space size={4}>
            <Typography.Link
              onClick={() => {
                setPayTarget(r);
                payForm.resetFields();
              }}
            >
              登记发放
            </Typography.Link>
            <Typography.Link
              onClick={() => {
                setAdjustTarget(r);
                setAdjustAmount(r.adjusted_amount || r.amount);
                setAdjustReason('');
              }}
            >
              调整金额
            </Typography.Link>
          </Space>
        ) : null,
    },
  ];

  return (
    <PageContainer title={false}>
      <Card
        size="small"
        title="奖金管理"
        extra={
          <Space>
            <ProFormSelect
              noStyle
              fieldProps={{ value: seasonId, onChange: (v: number) => setSeasonId(v), style: { minWidth: 140 } }}
              request={async () => (seasons ?? []).map((s) => ({ label: s.name, value: s.season_id }))}
            />
            <ProFormSelect
              noStyle
              allowClear
              fieldProps={{
                value: eventId,
                onChange: (v: number | undefined) => setEventId(v),
                style: { minWidth: 240 },
              }}
              request={async () =>
                (events?.list ?? []).map((e) => ({ label: e.name, value: e.event_id }))
              }
            />
            <Button onClick={() => setRulesOpen(true)}>奖金规则配置</Button>
          </Space>
        }
      >
        <Space size="large" style={{ marginBottom: 12 }}>
          <Statistic title="应发总额" value={formatMoney(summary?.total_amount ?? 0)} />
          <Statistic title="已发放" value={formatMoney(summary?.paid_amount ?? 0)} />
          <Statistic title="待发放笔数" value={summary?.unpaid_count ?? 0} />
        </Space>
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          title="默认规则：甲级赛每榜第 1 名（≥40 人 ¥2000 / <40 人 ¥1000）；超级赛每榜前 2 名（¥3000/1500 或 ¥2000/1000）；总决赛每榜前 8 名（¥10000/3000/2000/1000×5）；机构前 3 各 ¥10000。特殊场次可标记「不参与自动核算」。"
        />
        <ProTable<PrizeStatementItem>
          rowKey="item_id"
          size="small"
          search={false}
          options={false}
          pagination={false}
          actionRef={actionRef}
          dataSource={items}
          columns={columns}
        />
      </Card>

      <DrawerForm
        title="奖金规则配置（赛季级）"
        open={rulesOpen}
        onOpenChange={setRulesOpen}
        width={520}
        request={async () => (await getPrizeRules(seasonId)) as unknown as Record<string, unknown>}
        onFinish={async (values) => {
          await putPrizeRules(seasonId, values as unknown as PrizeRules);
          message.success('已保存');
          return true;
        }}
      >
          <Typography.Title level={5}>甲级赛（每榜第 1 名）</Typography.Title>
          <Space wrap>
            <ProFormDigit name={['grade_a', 'threshold']} label="人数阈值" min={0} width="xs" />
            <ProFormDigit name={['grade_a', 'above']} label="≥阈值金额（分）" min={0} width="sm" />
            <ProFormDigit name={['grade_a', 'below']} label="<阈值金额（分）" min={0} width="sm" />
          </Space>
          <Typography.Title level={5} style={{ marginTop: 16 }}>超级赛（每榜前 2 名）</Typography.Title>
          <Space wrap>
            <ProFormDigit name={['super', 'threshold']} label="人数阈值" min={0} width="xs" />
            <ProFormText name={['super', 'above']} label="≥阈值金额（分，逗号分隔两名）" width="md" />
            <ProFormText name={['super', 'below']} label="<阈值金额（分，逗号分隔两名）" width="md" />
          </Space>
          <Typography.Title level={5} style={{ marginTop: 16 }}>总决赛（每榜前 8 名金额，分）</Typography.Title>
          <ProFormText name={['final', 'amounts']} label="逗号分隔 8 档" width="lg" />
          <ProFormDigit name="institution_top3" label="机构前 3 名各（分）" min={0} width="sm" />
      </DrawerForm>

      <Modal
        title={payTarget ? `登记发放：${payTarget.player_name || payTarget.institution}` : ''}
        open={!!payTarget}
        onCancel={() => setPayTarget(null)}
        onOk={() => payForm.submit()}
      >
        <Form
          form={payForm}
          layout="vertical"
          onFinish={async (values) => {
            if (!payTarget) return;
            await payPrizeStatement(payTarget.item_id, {
              payee: values.payee,
              paid_at: values.paid_at?.toISOString?.() ?? new Date().toISOString(),
              remark: values.remark,
            });
            message.success('已登记发放');
            setPayTarget(null);
            refresh();
          }}
        >
          <Form.Item name="payee" label="领取人/代领人" rules={[{ required: true, message: '请填写领取人' }]}>
            <Input placeholder="如：家长代领 / 机构负责人" />
          </Form.Item>
          <Form.Item name="paid_at" label="发放日期" initialValue={null}>
            <DatePicker style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="remark" label="备注">
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={adjustTarget ? `调整金额：${adjustTarget.player_name || adjustTarget.institution}` : ''}
        open={!!adjustTarget}
        onCancel={() => setAdjustTarget(null)}
        onOk={async () => {
          if (!adjustTarget) return;
          if (!adjustReason.trim()) {
            message.warning('调整必须填写原因（留痕）');
            return;
          }
          await adjustPrizeStatement(adjustTarget.item_id, {
            amount: adjustAmount,
            reason: adjustReason,
          });
          message.success('已调整');
          setAdjustTarget(null);
          refresh();
        }}
      >
        <Alert type="warning" showIcon style={{ marginBottom: 12 }} message="人工微调金额需填原因，全程留痕。" />
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input
            type="number"
            addonBefore="金额（分）"
            value={adjustAmount}
            onChange={(e) => setAdjustAmount(Number(e.target.value))}
          />
          <Input.TextArea
            rows={3}
            placeholder="调整原因（必填）"
            value={adjustReason}
            onChange={(e) => setAdjustReason(e.target.value)}
          />
        </Space>
      </Modal>
    </PageContainer>
  );
};

export default Prizes;
