import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useNavigate } from "react-router-dom";
import { useState } from "react";
import { CheckCircle2, ExternalLink, Copy, Github, Shield } from "lucide-react";

const SetupCloudLoom = () => {
  const navigate = useNavigate();
  const [githubRepo, setGithubRepo] = useState("");
  const [roleArn, setRoleArn] = useState("");
  const [currentStep, setCurrentStep] = useState(1);

  const steps = [
    {
      id: 1,
      title: "Deploy CloudFormation Template",
      description: "Deploy the downloaded template to your AWS account",
      icon: <Shield className="w-6 h-6" />,
      content: (
        <div className="space-y-4">
          <div className="bg-blue-50 p-4 rounded-lg border border-blue-200">
            <h4 className="font-semibold text-blue-800 mb-2">AWS Console Deployment:</h4>
            <ol className="list-decimal list-inside space-y-2 text-sm text-blue-700">
              <li>Open the <a href="https://console.aws.amazon.com/cloudformation" target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline inline-flex items-center gap-1">AWS CloudFormation Console <ExternalLink className="w-3 h-3" /></a></li>
              <li>Click "Create stack" → "With new resources (standard)"</li>
              <li>Select "Upload a template file" and choose your downloaded YAML file</li>
              <li>Follow the wizard to configure stack parameters</li>
              <li>Review and create the stack</li>
              <li>Wait for deployment to complete (Status: CREATE_COMPLETE)</li>
            </ol>
          </div>
          <div className="bg-gray-50 p-4 rounded-lg border border-gray-200">
            <h4 className="font-semibold text-gray-800 mb-2">AWS CLI Alternative:</h4>
            <div className="bg-gray-900 text-green-400 p-3 rounded font-mono text-sm relative">
              <code>aws cloudformation create-stack --stack-name cloudloom-setup --template-body file://cloudformation-template.yaml --capabilities CAPABILITY_IAM</code>
              <Button 
                size="sm" 
                variant="ghost" 
                className="absolute top-2 right-2 text-gray-400 hover:text-white"
                onClick={() => navigator.clipboard.writeText('aws cloudformation create-stack --stack-name cloudloom-setup --template-body file://cloudformation-template.yaml --capabilities CAPABILITY_IAM')}
              >
                <Copy className="w-4 h-4" />
              </Button>
            </div>
          </div>
        </div>
      )
    },
    {
      id: 2,
      title: "Get Role ARN",
      description: "Copy the IAM Role ARN from CloudFormation outputs",
      icon: <CheckCircle2 className="w-6 h-6" />,
      content: (
        <div className="space-y-4">
          <div className="bg-yellow-50 p-4 rounded-lg border border-yellow-200">
            <h4 className="font-semibold text-yellow-800 mb-2">Find Your Role ARN:</h4>
            <ol className="list-decimal list-inside space-y-2 text-sm text-yellow-700">
              <li>Go to CloudFormation console and select your deployed stack</li>
              <li>Click on the "Outputs" tab</li>
              <li>Look for the output key named "CloudLoomRoleArn" or similar</li>
              <li>Copy the ARN value (format: arn:aws:iam::123456789012:role/CloudLoomRole)</li>
            </ol>
          </div>
          <div>
            <Label htmlFor="roleArn" className="text-base font-medium">Role ARN *</Label>
            <Input
              id="roleArn"
              placeholder="arn:aws:iam::123456789012:role/CloudLoomRole"
              value={roleArn}
              onChange={(e) => setRoleArn(e.target.value)}
              className="mt-2"
            />
          </div>
        </div>
      )
    },
    {
      id: 3,
      title: "Connect GitHub Repository",
      description: "Provide your GitHub repository for IaC scanning",
      icon: <Github className="w-6 h-6" />,
      content: (
        <div className="space-y-4">
          <div className="bg-purple-50 p-4 rounded-lg border border-purple-200">
            <h4 className="font-semibold text-purple-800 mb-2">Repository Requirements:</h4>
            <ul className="list-disc list-inside space-y-1 text-sm text-purple-700">
              <li>Repository should contain Infrastructure as Code files</li>
              <li>Supported: CloudFormation, Terraform, Kubernetes manifests</li>
              <li>CloudLoom will scan for security misconfigurations</li>
              <li>Repository must be accessible (public or with proper permissions)</li>
            </ul>
          </div>
          <div>
            <Label htmlFor="githubRepo" className="text-base font-medium">GitHub Repository URL *</Label>
            <Input
              id="githubRepo"
              placeholder="https://github.com/yourusername/your-repo"
              value={githubRepo}
              onChange={(e) => setGithubRepo(e.target.value)}
              className="mt-2"
            />
          </div>
        </div>
      )
    }
  ];

  const handleNext = () => {
    if (currentStep < steps.length) {
      setCurrentStep(currentStep + 1);
    }
  };

  const handlePrevious = () => {
    if (currentStep > 1) {
      setCurrentStep(currentStep - 1);
    }
  };

  const handleComplete = () => {
    if (!roleArn || !githubRepo) {
      alert("Please fill in both the Role ARN and GitHub Repository URL");
      return;
    }
    
    // Here you could make an API call to save the configuration
    console.log("Configuration:", { roleArn, githubRepo });
    
    // Navigate to dashboard
    navigate("/dashboard");
  };

  const isStepComplete = (stepId: number) => {
    if (stepId === 2) return roleArn.length > 0;
    if (stepId === 3) return githubRepo.length > 0;
    return stepId < currentStep;
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-indigo-50 to-purple-100 relative overflow-hidden">
      {/* Decorative texture */}
      <div className="absolute inset-0 pointer-events-none z-0" style={{
        backgroundImage: `url('https://www.transparenttextures.com/patterns/cubes.png')`,
        opacity: 0.1,
        mixBlendMode: 'multiply',
      }} />
      
      <div className="relative z-10 container mx-auto px-4 py-8">
        <div className="text-center mb-8">
          <h1 className="text-4xl font-bold text-indigo-700 mb-2">Setup CloudLoom</h1>
          <p className="text-lg text-gray-600">Complete these steps to get CloudLoom monitoring your infrastructure</p>
        </div>

        {/* Progress indicator */}
        <div className="max-w-4xl mx-auto mb-8">
          <div className="flex items-center justify-between">
            {steps.map((step, index) => (
              <div key={step.id} className="flex items-center">
                <div className={`flex items-center justify-center w-12 h-12 rounded-full border-2 transition-all ${
                  isStepComplete(step.id) 
                    ? 'bg-green-500 border-green-500 text-white' 
                    : currentStep === step.id 
                      ? 'bg-indigo-500 border-indigo-500 text-white' 
                      : 'bg-white border-gray-300 text-gray-400'
                }`}>
                  {isStepComplete(step.id) ? <CheckCircle2 className="w-6 h-6" /> : step.icon}
                </div>
                {index < steps.length - 1 && (
                  <div className={`w-24 h-1 mx-4 ${
                    isStepComplete(step.id) ? 'bg-green-500' : 'bg-gray-300'
                  }`} />
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Step content */}
        <div className="max-w-4xl mx-auto">
          <Card className="p-8 shadow-xl bg-white">
            <div className="mb-6">
              <h2 className="text-2xl font-bold text-gray-800 mb-2">
                Step {currentStep}: {steps[currentStep - 1].title}
              </h2>
              <p className="text-gray-600">{steps[currentStep - 1].description}</p>
            </div>
            
            {steps[currentStep - 1].content}

            {/* Navigation buttons */}
            <div className="flex justify-between mt-8">
              <Button 
                variant="outline" 
                onClick={handlePrevious}
                disabled={currentStep === 1}
              >
                Previous
              </Button>
              
              {currentStep === steps.length ? (
                <Button 
                  onClick={handleComplete}
                  className="bg-green-600 hover:bg-green-700"
                  disabled={!roleArn || !githubRepo}
                >
                  Complete Setup
                </Button>
              ) : (
                <Button onClick={handleNext}>
                  Next
                </Button>
              )}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
};

export default SetupCloudLoom;
