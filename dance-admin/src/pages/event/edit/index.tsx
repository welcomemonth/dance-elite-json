import {
  PageContainer,
  ProForm,
  ProFormDigit,
  ProFormList,
  ProFormRadio,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import { history, useParams } from '@umijs/max';
import { Alert, App, Button, Card, DatePicker, Form, Space, Typography } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import React, { useEffect, useState } from 'react';
import {
  createEvent,
  getEvent,
  updateEvent,
} from '@/services/youwumeng/events';
import { listSeasons } from '@/services/youwumeng/seasons';
import {
  AGE_LEVEL_OPTIONS,
  DANCE_TYPE_LABELS,
  EVENT_LEVEL_LABELS,
} from '@/services/youwumeng/labels';
import type {
  AgeLevel,
  DanceType,
  EventInput,
  EventLevel,
} from '@/services/youwumeng/typings';

/** 系统规则：出生年份范围（截止日 2027-01-01），只读展示 */
const AGE_YEAR_RANGES: Record<AgeLevel, string> = {
  U11: '2016–2018',
  U13: '2014–2015',
  U15: '2012–2013',
};

const DEFAULT_REFUND_POLICY =
  '抽签前可无偿退赛，全额原路退回；抽签后个人原因退赛扣除 50% 报名费；伤病凭医院诊断证明可顺延不扣费。所有退款原路退回。';

const TIMELINE_FIELDS = [
  { name: 'reg_start_at', label: '报名开始' },
  { name: 'reg_end_at', label: '截止报名' },
  { name: 'lottery_at', label: '抽签' },
  { name: 'material_check_at', label: '核对舞蹈音频（资料上传最终期限）' },
  { name: 'start_at', label: '开始比赛' },
] as const;

type TimelineField = (typeof TIMELINE_FIELDS)[number]['name'];

interface FormValues {
  name: string;
  season_id: number;
  level: EventLevel;
  city?: string;
  venue: string;
  event_date: Dayjs;
  fee_yuan: number;
  reg_start_at: Dayjs;
  reg_end_at: Dayjs;
  lottery_at: Dayjs;
  material_check_at: Dayjs;
  start_at: Dayjs;
  groups: { group_id?: number; level: AgeLevel; dance_type: DanceType; quota: number }[];
  refund_policy?: string;
  work_duration_limit?: string;
  process_desc?: string;
  honor_desc?: string;
  exclude_from_prize?: boolean;
}

const toIso = (d: Dayjs): string => d.format('YYYY-MM-DDTHH:mm:ssZ');

const EventEditForm: React.FC = () => {
  const params = useParams<{ id?: string }>();
  const eventId = params.id ? Number(params.id) : undefined;
  const isEdit = eventId !== undefined;
  const { message } = App.useApp();
  const [form] = ProForm.useForm<FormValues>();
  const [published, setPublished] = useState(false);
  const [seasonOptions, setSeasonOptions] = useState<
    { label: string; value: number }[]
  >([]);
  const [saving, setSaving] = useState(false);
  /** 「保存并发布」标记：在 form.submit() 前置位，onFinish 读取 */
  const publishRef = React.useRef(false);

  const level = Form.useWatch('level', form);

  useEffect(() => {
    listSeasons().then((list) =>
      setSeasonOptions(
        list.map((s) => ({ label: s.name, value: s.season_id })),
      ),
    );
  }, []);

  useEffect(() => {
    if (!isEdit) return;
    getEvent(eventId)
      .then((event) => {
        setPublished(!['draft', 'unpublished'].includes(event.status));
        form.setFieldsValue({
          name: event.name,
          season_id: event.season_id,
          level: event.level,
          city: event.city,
          venue: event.venue,
          event_date: dayjs(event.event_date),
          fee_yuan: event.fee / 100,
          reg_start_at: dayjs(event.timeline.reg_start_at),
          reg_end_at: dayjs(event.timeline.reg_end_at),
          lottery_at: dayjs(event.timeline.lottery_at),
          material_check_at: dayjs(event.timeline.material_check_at),
          start_at: dayjs(event.timeline.start_at),
          groups: event.groups.map((g) => ({
            group_id: g.group_id,
            level: g.level,
            dance_type: g.dance_type,
            quota: g.quota,
          })),
          refund_policy: event.refund_policy,
          work_duration_limit: event.work_duration_limit,
          process_desc: event.process_desc,
          honor_desc: event.honor_desc,
          exclude_from_prize: event.exclude_from_prize,
        });
      })
      .catch(() => {
        message.error('赛事不存在或加载失败');
        history.replace('/event/list');
      });
  }, [eventId, form, isEdit, message]);

  const validateTimeline = (values: FormValues): string | null => {
    for (let i = 1; i < TIMELINE_FIELDS.length; i += 1) {
      const prev = values[TIMELINE_FIELDS[i - 1].name];
      const curr = values[TIMELINE_FIELDS[i].name];
      if (!prev || !curr || !curr.isAfter(prev)) {
        return `${TIMELINE_FIELDS[i].label} 须晚于「${TIMELINE_FIELDS[i - 1].label}」`;
      }
    }
    return null;
  };

  const onFinish = async (values: FormValues) => {
    const timelineError = validateTimeline(values);
    if (timelineError) {
      message.error(timelineError);
      return;
    }
    const seen = new Set<string>();
    for (const g of values.groups) {
      const key = `${g.level}:${g.dance_type}`;
      if (seen.has(key)) {
        message.error(`组别配置重复：${g.level} × ${DANCE_TYPE_LABELS[g.dance_type]}`);
        return;
      }
      seen.add(key);
    }

    const input: EventInput = {
      name: values.name,
      season_id: values.season_id,
      level: values.level,
      city: values.city,
      venue: values.venue,
      event_date: values.event_date.format('YYYY-MM-DD'),
      fee: Math.round(values.fee_yuan * 100),
      timeline: {
        reg_start_at: toIso(values.reg_start_at),
        reg_end_at: toIso(values.reg_end_at),
        lottery_at: toIso(values.lottery_at),
        material_check_at: toIso(values.material_check_at),
        start_at: toIso(values.start_at),
      },
      groups: values.groups.map((g) => ({
        group_id: g.group_id,
        level: g.level,
        dance_type: g.dance_type,
        quota: g.quota,
      })),
      refund_policy: values.refund_policy,
      work_duration_limit: values.work_duration_limit,
      process_desc: values.process_desc,
      honor_desc: values.honor_desc,
      exclude_from_prize: values.exclude_from_prize,
    };

    setSaving(true);
    try {
      if (isEdit) {
        await updateEvent(eventId, input);
        message.success('保存成功');
      } else if (publishRef.current) {
        await createEvent({ ...input, publish: true });
        message.success('已创建并发布，小程序端立即可见');
      } else {
        await createEvent({ ...input, publish: false });
        message.success('草稿已保存');
      }
      history.push('/event/list');
    } finally {
      publishRef.current = false;
      setSaving(false);
    }
  };

  const submitWithPublish = (publish: boolean) => {
    publishRef.current = publish;
    form.submit();
  };

  const timelineDisabled = published;
  const levelOptions = Object.entries(EVENT_LEVEL_LABELS).map(([value, label]) => ({
    value,
    label,
  }));
  const ageOptions = AGE_LEVEL_OPTIONS.map((v) => ({ value: v, label: v }));
  const danceOptions = Object.entries(DANCE_TYPE_LABELS).map(([value, label]) => ({
    value,
    label,
  }));

  return (
    <PageContainer
      title={isEdit ? '编辑赛事' : '新建赛事'}
      header={{
        breadcrumb: {
          items: [
            { title: '赛事管理' },
            { title: '赛事列表', path: '/event/list' },
            { title: isEdit ? '编辑赛事' : '新建赛事' },
          ],
        },
      }}
    >
      <ProForm<FormValues>
        form={form}
        onFinish={onFinish}
        submitter={false}
        initialValues={{
          fee_yuan: 980,
          exclude_from_prize: false,
          refund_policy: DEFAULT_REFUND_POLICY,
          work_duration_limit: '作品时长 ≤3 分钟',
          process_desc: '常规赛 → 前 20% 晋级决赛 → 决赛 → 当天颁奖',
        }}
      >
        <Card title="基本信息" style={{ marginBottom: 16 }}>
          <ProFormText
            name="name"
            label="赛事名称"
            rules={[{ required: true, message: '请输入赛事名称' }]}
          />
          <ProFormSelect
            name="level"
            label="级别"
            options={levelOptions}
            disabled={published}
            rules={[{ required: true, message: '请选择级别' }]}
            fieldProps={{
              onChange: (value) => {
                if (value === 'final') {
                  form.setFieldValue('fee_yuan', 0);
                }
              },
            }}
          />
          <ProFormSelect
            name="season_id"
            label="所属赛季"
            options={seasonOptions}
            rules={[{ required: true, message: '请选择赛季' }]}
          />
          <ProFormText name="city" label="举办城市" />
          <ProFormText
            name="venue"
            label="比赛地点"
            rules={[{ required: true, message: '请输入比赛地点' }]}
          />
          <ProForm.Item
            name="event_date"
            label="比赛日期"
            rules={[{ required: true, message: '请选择比赛日期' }]}
          >
            <DatePicker style={{ width: 240 }} />
          </ProForm.Item>
          <ProFormDigit
            name="fee_yuan"
            label="报名费（元）"
            min={0}
            precision={2}
            disabled={level === 'final'}
            fieldProps={{ style: { width: 200 } }}
            rules={[{ required: true, message: '请输入报名费' }]}
            tooltip={level === 'final' ? '年度总决赛默认为免费' : undefined}
          />
        </Card>

        <Card title="赛事时间线" style={{ marginBottom: 16 }}>
          <Alert
            type="info"
            showIcon
            title="截止报名后不可再报名/退款；任一节点须晚于前一节点。"
            style={{ marginBottom: 16 }}
          />
          {TIMELINE_FIELDS.map((field, index) => (
            <ProForm.Item
              key={field.name}
              name={field.name}
              label={field.label}
              disabled={timelineDisabled}
              rules={[
                { required: true, message: `请选择${field.label}时间` },
                {
                  validator: (_: unknown, value: Dayjs | undefined) => {
                    if (!value || index === 0) return Promise.resolve();
                    const prev = form.getFieldValue(
                      TIMELINE_FIELDS[index - 1].name,
                    ) as Dayjs | undefined;
                    if (prev && !value.isAfter(prev)) {
                      return Promise.reject(
                        new Error(`${field.label} 须晚于「${TIMELINE_FIELDS[index - 1].label}」`),
                      );
                    }
                    return Promise.resolve();
                  },
                },
              ]}
            >
              <DatePicker showTime style={{ width: 240 }} />
            </ProForm.Item>
          ))}
        </Card>

        <Card title="组别配置" style={{ marginBottom: 16 }}>
          <Typography.Paragraph type="secondary">
            年龄组按出生年自动判定（系统规则，不可修改）：U11 对应{' '}
            {AGE_YEAR_RANGES.U11}、U13 对应 {AGE_YEAR_RANGES.U13}、U15 对应{' '}
            {AGE_YEAR_RANGES.U15}（出生年份截止日 2027-01-01）。
          </Typography.Paragraph>
          <ProFormList
            name="groups"
            min={1}
            copyIconProps={false}
            creatorButtonProps={{
              creatorButtonText: '添加组别 × 舞种',
            }}
          >
            <Space align="baseline" wrap>
              <ProFormSelect
                name="level"
                label="组别"
                options={ageOptions}
                rules={[{ required: true, message: '请选择组别' }]}
                fieldProps={{ style: { width: 100 } }}
              />
              <ProFormSelect
                name="dance_type"
                label="舞种"
                options={danceOptions}
                rules={[{ required: true, message: '请选择舞种' }]}
                fieldProps={{ style: { width: 140 } }}
              />
              <ProFormDigit
                name="quota"
                label="名额"
                min={1}
                fieldProps={{ style: { width: 120 } }}
                rules={[{ required: true, message: '请输入名额' }]}
              />
            </Space>
          </ProFormList>
        </Card>

        <Card title="规则与说明文案" style={{ marginBottom: 16 }}>
          <ProFormTextArea
            name="refund_policy"
            label="退费政策"
            rows={3}
            placeholder={DEFAULT_REFUND_POLICY}
          />
          <ProFormTextArea
            name="work_duration_limit"
            label="作品时长要求"
            rows={2}
          />
          <ProFormTextArea
            name="process_desc"
            label="比赛流程说明"
            rows={2}
          />
          <ProFormTextArea
            name="honor_desc"
            label="荣誉与奖金说明"
            rows={2}
          />
          <ProFormRadio.Group
            name="exclude_from_prize"
            label="特殊场次"
            radioType="button"
            options={[
              { value: false, label: '参与自动奖金核算' },
              { value: true, label: '不参与自动奖金核算（特殊场次）' },
            ]}
          />
        </Card>

        <Space>
          {isEdit ? (
            <>
              <Button type="primary" htmlType="submit" loading={saving}>
                保存
              </Button>
              <Button onClick={() => history.back()}>取消</Button>
            </>
          ) : (
            <>
              <Button loading={saving} onClick={() => submitWithPublish(false)}>
                保存草稿
              </Button>
              <Button
                type="primary"
                loading={saving}
                onClick={() => {
                  form
                    .validateFields()
                    .then(() => {
                      submitWithPublish(true);
                    })
                    .catch(() => undefined);
                }}
              >
                保存并发布
              </Button>
              <Button onClick={() => history.back()}>取消</Button>
            </>
          )}
        </Space>
      </ProForm>
    </PageContainer>
  );
};

export default EventEditForm;
