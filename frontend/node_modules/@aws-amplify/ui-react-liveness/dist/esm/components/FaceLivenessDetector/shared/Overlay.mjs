import * as React from 'react';
import { Flex } from '@aws-amplify/ui-react';
import { LivenessClassNames } from '../types/classNames.mjs';

const Overlay = ({ children, horizontal = 'center', vertical = 'center', className, ...rest }) => {
    return (React.createElement(Flex, { className: `${LivenessClassNames.Overlay} ${className}`, alignItems: horizontal, justifyContent: vertical, ...rest }, children));
};

export { Overlay };
