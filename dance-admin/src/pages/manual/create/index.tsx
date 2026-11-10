import {
  PageContainer,
  ProForm,
  ProFormDigit,
  ProFormRadio,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import { history } from '@umijs/max';
import { useService } from '@/services/youwumeng/useService';
import { App, Alert, Form } from 'antd';
import React, { useState } from 'react';
import { getEvent, listEvents } from '@/services/youwumeng/events';
import {
  CHARGE_TYPE_LABELS,
  DANCE_TYPE_LABELS,
  ID_TYPE_LABELS,
} from '@/services/youwumeng/labels';
import { createManualRegistration } from '@/services/youwumeng/waitlists';
import type { ChargeType, EventBrief, ManualRegistrationInput } from '@/services/youwumeng/typings';

/**
 * 新建人工报名（特殊通道）：用于无大陆身份证号选手。
 * 名额校验与小程序一致（服务端 41005/41006，组别下拉前置禁用满员/关闭）。
 * 对应设计文档 5.2 节。
 */
const ManualCreate: React.FC = () => {
  const { message } = App.useApp();
  const [form] = Form.useForm<ManualRegistrationInput & { event_id: number }>();
  const chargeType = Form.useWatch('charge_type', form);
  const eventId = Form.useWatch('event_id', form);
  const [submitting, setSubmitting] = useState(false);

  // 可人工报名的赛事：报名中/未发布（其余状态服务端会 41006）
  const { data: eventOptions } = useService(async () => {
    const res = await listEvents({ page: 1, page_size: 100 });
    return res.list
      .filter((e: EventBrief) => ['registering', 'unpublished'].includes(e.status))
      .map((e: EventBrief) => ({ label: e.name, value: e.event_id }));
  });

  const { data: groupOptions } = useService(
    async () => {
      if (!eventId) return [];
      const event = await getEvent(eventId);
      return event.groups.map((g) => {
        const full = g.registered >= g.quota;
        const closed = !g.enabled;
        return {
          label: `${g.level}·${DANCE_TYPE_LABELS[g.dance_type]}（已报 ${g.registered}/${g.quota}）${closed ? '（已关闭）' : full ? '（已满员）' : ''}`,
          value: g.group_id,
          disabled: closed || full,
        };
      });
    },
    { refreshDeps: [eventId], ready: !!eventId },
  );

  const handleFinish = async (values: ManualRegistrationInput & { event_id: number }) => {
    setSubmitting(true);
    try {
      const result = await createManualRegistration({
        event_id: values.event_id,
        group_id: values.group_id,
        player_name: values.player_name,
        id_type: values.id_type,
        id_card_no: values.id_card_no,
        nationality: values.nationality,
        institution: values.institution,
        contact_phone: values.contact_phone,
        charge_type: values.charge_type,
        // 表单金额为元，接口金额为分
        amount: values.charge_type === 'free' ? 0 : Math.round((values.amount ?? 0) * 100),
        cs_ticket_id: values.cs_ticket_id,
        remark: values.remark,
      });
      message.success(`人工报名成功，订单号 ${result.order_no}`);
      history.push('/order/manual');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <PageContainer title="新建人工报名">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16, maxWidth: 720 }}
        title="特殊通道：用于无中国大陆身份证号的选手（国际友人等）。订单直接置「已支付」，不产生在线支付；名额校验与小程序一致。"
      />
      <ProForm
        form={form}
        layout="vertical"
        style={{ maxWidth: 720 }}
        submitter={{ searchConfig: { submitText: '提交报名' } }}
        onFinish={handleFinish}
      >
        <ProFormSelect
          name="event_id"
          label="赛事"
          placeholder="选择报名中/未开始的赛事"
          options={eventOptions}
          rules={[{ required: true, message: '请选择赛事' }]}
          fieldProps={{ onChange: () => form.setFieldValue('group_id', undefined) }}
        />
        <ProFormSelect
          name="group_id"
          label="组别（手选，需人工确认年龄符合）"
          placeholder={eventId ? '选择组别' : '请先选择赛事'}
          options={groupOptions}
          disabled={!eventId}
          rules={[{ required: true, message: '请选择组别' }]}
        />
        <ProFormText
          name="player_name"
          label="选手姓名"
          rules={[{ required: true, message: '请输入选手姓名' }]}
        />
        <ProFormSelect
          name="id_type"
          label="证件类型"
          options={Object.entries(ID_TYPE_LABELS).map(([value, label]) => ({ value, label }))}
          rules={[{ required: true, message: '请选择证件类型' }]}
        />
        <ProFormText
          name="id_card_no"
          label="证件号"
          tooltip="加密存储，不与大陆身份证判重库混用"
          rules={[{ required: true, message: '请输入证件号' }]}
        />
        <ProFormText name="nationality" label="国籍/地区" />
        <ProFormText name="institution" label="机构名称" />
        <ProFormText
          name="contact_phone"
          label="联系电话"
          tooltip="用于候补通知/赛事联系"
          rules={[
            { required: true, message: '请输入联系电话' },
            { pattern: /^1\d{10}$/, message: '手机号格式错误' },
          ]}
        />
        <ProFormRadio.Group
          name="charge_type"
          label="收费方式"
          initialValue="free"
          options={Object.entries(CHARGE_TYPE_LABELS).map(([value, label]) => ({ value, label }))}
          rules={[{ required: true }]}
        />
        {chargeType === ('offline_paid' as ChargeType) && (
          <ProFormDigit
            name="amount"
            label="线下收款金额（元）"
            min={0.01}
            fieldProps={{ precision: 2 }}
            rules={[{ required: true, message: '请填写线下收款金额' }]}
          />
        )}
        <ProFormDigit name="cs_ticket_id" label="关联客服工单号" min={1} />
        <ProFormTextArea name="remark" label="备注" placeholder="特殊情况说明" />
      </ProForm>
    </PageContainer>
  );
};

export default ManualCreate;
