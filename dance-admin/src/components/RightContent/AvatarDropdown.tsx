import { LogoutOutlined } from '@ant-design/icons';
import { history, useModel } from '@umijs/max';
import type { MenuProps } from 'antd';
import { Spin } from 'antd';
import React, { startTransition } from 'react';
import { clearSession } from '@/services/youwumeng/auth';
import HeaderDropdown from '../HeaderDropdown';

type GlobalHeaderRightProps = {
  children?: React.ReactNode;
};

const menuItems: MenuProps['items'] = [
  {
    key: 'logout',
    icon: <LogoutOutlined />,
    label: '退出登录',
  },
];

export const AvatarDropdown: React.FC<GlobalHeaderRightProps> = ({ children }) => {
  const { initialState, setInitialState } = useModel('@@initialState');

  const onMenuClick: MenuProps['onClick'] = (event) => {
    if (event.key !== 'logout') return;
    // 后端无登出接口（JWT 短期有效），本地清除会话即可
    clearSession();
    startTransition(() => {
      setInitialState((s) => ({ ...s, currentUser: undefined }));
    });
    const { search, pathname } = window.location;
    if (pathname !== '/user/login') {
      history.replace({
        pathname: '/user/login',
        search: new URLSearchParams({ redirect: pathname + search }).toString(),
      });
    }
  };

  if (!initialState?.currentUser) {
    return <Spin size="small" />;
  }

  const { currentUser } = initialState;

  return (
    <HeaderDropdown
      placement="bottomRight"
      menu={{ selectedKeys: [], onClick: onMenuClick, items: menuItems }}
      arrow
    >
      {children}
    </HeaderDropdown>
  );
};
