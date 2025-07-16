import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import Index from "./pages/Index";
import Dashboard from "./pages/Dashboard";
import RiskAssessmentPage from "./pages/RiskAssessment";
import AccessControlPage from "./pages/AccessControl";
import NotFound from "./pages/NotFound";
import Terraform from "./pages/Terraform";
import Report from "./pages/Report";
import CloudLoomWelcome from "./pages/CloudLoomWelcome";
import ChoosePlan from "./pages/ChoosePlan";
import SetupCloudLoom from "./pages/SetupCloudLoom";

const queryClient = new QueryClient();

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster />
      <Sonner />
      <BrowserRouter>
        {/* Only show sidebar for non-welcome and non-choose-plan routes */}
        <Routes>
          <Route path="/" element={<CloudLoomWelcome />} />
          <Route path="/choose-plan" element={<ChoosePlan />} />
          <Route path="/setup" element={<SetupCloudLoom />} />
          <Route
            path="*"
            element={
              <SidebarProvider defaultOpen={true}>
                <div className="flex w-full min-h-screen">
                  <AppSidebar />
                  <div className="flex-1">
                    <Routes>
                      <Route path="/dashboard" element={<Dashboard />} />
                      <Route path="/resource-graph" element={<Index />} />
                      <Route path="/risks" element={<RiskAssessmentPage />} />
                      <Route path="/access" element={<AccessControlPage />} />
                      <Route path="/report/:version" element={<Report />} />
                      <Route path="/terraform/:version" element={<Terraform />} />
                      <Route path="*" element={<NotFound />} />
                    </Routes>
                  </div>
                </div>
              </SidebarProvider>
            }
          />
        </Routes>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;