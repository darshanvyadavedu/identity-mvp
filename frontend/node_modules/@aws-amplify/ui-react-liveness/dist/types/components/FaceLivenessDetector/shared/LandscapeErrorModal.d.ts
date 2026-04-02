import * as React from 'react';
interface LandscapeErrorModalProps {
    onRetry: () => void;
    header: string;
    portraitMessage: string;
    landscapeMessage: string;
    tryAgainText: string;
}
export declare const LandscapeErrorModal: React.FC<LandscapeErrorModalProps>;
export {};
