import { createStyles } from 'antd-style';
import React from 'react';

const useStyles = createStyles(({ token, css }) => ({
  footer: css`
    padding: 16px 24px;
    text-align: center;
    color: ${token.colorTextDescription};
    font-size: ${token.fontSizeSM}px;
    background: transparent;
  `,
}));

const Footer: React.FC = () => {
  const { styles } = useStyles();
  const year = new Date().getFullYear();
  return <div className={styles.footer}>优舞盟后台管理系统 &copy; {year}</div>;
};

export default Footer;
