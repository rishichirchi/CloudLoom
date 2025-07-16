import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { useNavigate } from "react-router-dom";
import { useState } from "react";

const plans = [
	{
		name: "Notification Tier: CloudInsight",
		description: "Proactive monitoring and alerting for cloud vulnerabilities.",
		color: "border-blue-400",
		accessTier: "CloudLoomNotificationTier",
		features: [
			"Comprehensive log analysis",
			"IaC scanning for misconfigurations",
			"Real-time vulnerability detection",
			"Contextual alerting",
		],
	},
	{
		name: "Suggest Fix Tier: CloudAdvisor",
		description: "Get actionable recommendations to resolve detected issues.",
		color: "border-indigo-400",
		accessTier: "CloudLoomSuggestFixTier",
		features: [
			"All Notification Tier features",
			"AI-driven remediation suggestions",
			"IaC and runtime fix guidance",
			"Impact analysis simulation",
		],
	},
	{
		name: "Auto Apply Fix Tier: CloudGuardian",
		description: "Automate the resolution of vulnerabilities with AI-driven fixes.",
		color: "border-green-400",
		accessTier: "CloudLoomAutoApplyFixTier",
		features: [
			"All Notification & Suggest Fix features",
			"Automated remediation execution",
			"GitHub PR automation",
			"Continuous validation & audit",
		],
	},
];

const ChoosePlan = () => {
	const navigate = useNavigate();
	const [loadingTier, setLoadingTier] = useState<string | null>(null);

	const handleSelectPlan = async (accessTier: string) => {
		setLoadingTier(accessTier);
		try {
			const response = await fetch('http://localhost:5000/api/v1/cloudformation/download-template', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					accessTier: accessTier,
				}),
			});

			if (!response.ok) {
				throw new Error(`HTTP error! Status: ${response.status}`);
			}

			// Get the filename from the response headers or use a default name
			const contentDisposition = response.headers.get('content-disposition');
			let filename = 'cloudformation-template.yaml';
			if (contentDisposition) {
				const filenameMatch = contentDisposition.match(/filename="?([^"]+)"?/);
				if (filenameMatch) {
					filename = filenameMatch[1];
				}
			}

			// Create blob and download file
			const blob = await response.blob();
			const url = window.URL.createObjectURL(blob);
			const link = document.createElement('a');
			link.href = url;
			link.download = filename;
			document.body.appendChild(link);
			link.click();
			document.body.removeChild(link);
			window.URL.revokeObjectURL(url);

			// Navigate to setup page after successful download
			navigate("/setup");
		} catch (error) {
			console.error('Error downloading CloudFormation template:', error);
			alert('Failed to download CloudFormation template. Please try again.');
		} finally {
			setLoadingTier(null);
		}
	};

	return (
		<div className="flex flex-col items-center justify-center min-h-screen bg-gradient-to-br from-white to-blue-50 relative overflow-hidden">
			{/* Decorative texture */}
			<div className="absolute inset-0 pointer-events-none z-0" style={{
				backgroundImage: `url('https://www.transparenttextures.com/patterns/cubes.png')`,
				opacity: 0.12,
				mixBlendMode: 'multiply',
			}} />
			<h2 className="text-4xl font-bold text-indigo-700 mb-8 z-10 relative">Choose Your Plan</h2>
			<div className="flex gap-8 flex-wrap justify-center z-10 relative">
				{plans.map((plan, idx) => (
					<Card key={plan.name} className={`w-96 p-8 border-2 ${plan.color} shadow-xl flex flex-col items-center transition-transform hover:scale-105 bg-white rounded-xl`}>
						<h3 className="text-2xl font-bold mb-2 text-indigo-600 text-center">{plan.name}</h3>
						<p className="text-gray-700 mb-4 text-center text-base font-medium">{plan.description}</p>
						<ul className="list-disc list-inside text-sm text-gray-600 mb-6 text-left w-full">
							{plan.features.map((feature, fidx) => (
								<li key={fidx} className="mb-2">{feature}</li>
							))}
						</ul>
						<Button 
							size="lg" 
							className="w-full mt-auto" 
							onClick={() => handleSelectPlan(plan.accessTier)}
							disabled={loadingTier === plan.accessTier}
						>
							{loadingTier === plan.accessTier ? "Downloading..." : "Select"}
						</Button>
					</Card>
				))}
			</div>
		</div>
	);
};

export default ChoosePlan;
